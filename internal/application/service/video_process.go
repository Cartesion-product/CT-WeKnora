package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/access"
	"github.com/Tencent/WeKnora/internal/application/service/retriever"
	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/media"
	"github.com/Tencent/WeKnora/internal/models/asr"
	"github.com/Tencent/WeKnora/internal/models/vlm"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const videoProcessorVersion = "video-v1"

type VideoProcessPayload struct {
	TenantID        uint64 `json:"tenant_id"`
	KnowledgeID     string `json:"knowledge_id"`
	ProcessingRunID string `json:"processing_run_id"`
}

type VideoProcessor struct {
	db        *gorm.DB
	knowledge interfaces.KnowledgeRepository
	kbs       interfaces.KnowledgeBaseService
	tenants   interfaces.TenantRepository
	files     interfaces.FileService
	storage   interfaces.StorageBackendResolver
	models    interfaces.ModelService
	indexer   interfaces.KnowledgeService
}

func NewVideoProcessor(db *gorm.DB, knowledge interfaces.KnowledgeRepository,
	kbs interfaces.KnowledgeBaseService, tenants interfaces.TenantRepository,
	files interfaces.FileService, models interfaces.ModelService,
	indexer interfaces.KnowledgeService, storage interfaces.StorageBackendResolver) *VideoProcessor {
	return &VideoProcessor{db: db, knowledge: knowledge, kbs: kbs, tenants: tenants,
		files: files, models: models, indexer: indexer, storage: storage}
}

// Handle resumes individual ASR windows after a retry. Only the run named in
// the payload can publish; a newer upload or rebuild supersedes it.
func (p *VideoProcessor) Handle(ctx context.Context, task *asynq.Task) (err error) {
	var payload VideoProcessPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil || payload.TenantID == 0 ||
		payload.KnowledgeID == "" || payload.ProcessingRunID == "" {
		return fmt.Errorf("invalid video task payload: %w", asynq.SkipRetry)
	}
	ctx = types.WithExecutionTenant(ctx, payload.TenantID)
	tenant, err := p.tenants.GetTenantByID(ctx, payload.TenantID)
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, types.TenantInfoContextKey, tenant)
	knowledge, err := p.knowledge.GetKnowledgeByID(ctx, payload.TenantID, payload.KnowledgeID)
	if err != nil || knowledge == nil || !IsVideoType(knowledge.FileType) {
		return fmt.Errorf("video knowledge unavailable: %w", asynq.SkipRetry)
	}
	kb, err := p.kbs.GetKnowledgeBaseByID(ctx, knowledge.KnowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID != payload.TenantID {
		return fmt.Errorf("video knowledge base unavailable: %w", asynq.SkipRetry)
	}
	ctx, err = access.WithKBTaskWrite(ctx, kb, payload.TenantID)
	if err != nil {
		return fmt.Errorf("video task scope: %w", asynq.SkipRetry)
	}
	var asset types.VideoAsset
	if err := p.db.WithContext(ctx).Where("knowledge_id = ? AND tenant_id = ?", knowledge.ID, payload.TenantID).First(&asset).Error; err != nil {
		return err
	}
	if asset.CurrentRunID != payload.ProcessingRunID || asset.FilePath != knowledge.FilePath ||
		knowledge.ParseStatus == types.ParseStatusDeleting || knowledge.ParseStatus == types.ParseStatusCancelled {
		return nil
	}
	var run types.VideoRun
	if err := p.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND knowledge_id = ?",
		payload.ProcessingRunID, payload.TenantID, knowledge.ID).First(&run).Error; err != nil {
		return err
	}
	if run.Status == "completed" {
		return nil
	}
	if asset.ActiveRunID == "" {
		if err := p.db.WithContext(ctx).Model(&types.Knowledge{}).
			Where("id = ? AND tenant_id = ? AND parse_status NOT IN ?", knowledge.ID, payload.TenantID,
				[]string{types.ParseStatusDeleting, types.ParseStatusCancelled}).
			Updates(map[string]any{"parse_status": types.ParseStatusProcessing, "error_message": ""}).Error; err != nil {
			return err
		}
	}
	defer func() {
		if err != nil && !errors.Is(err, asynq.SkipRetry) && ctx.Err() == nil {
			failureCtx := context.WithoutCancel(ctx)
			p.db.WithContext(failureCtx).Model(&types.VideoRun{}).
				Where("id = ? AND status <> ?", run.ID, "cancelled").Updates(map[string]any{"status": "failed", "failure_code": "VIDEO_PROCESS_FAILED"})
			p.db.WithContext(failureCtx).Model(&types.VideoAsset{}).
				Where("knowledge_id = ? AND current_run_id = ? AND status <> ?", knowledge.ID, run.ID, "cancelled").
				Updates(map[string]any{"status": "failed", "failure_code": "VIDEO_PROCESS_FAILED"})
			if asset.ActiveRunID == "" {
				p.db.WithContext(failureCtx).Model(&types.Knowledge{}).
					Where("id = ? AND tenant_id = ? AND parse_status NOT IN ?", knowledge.ID, run.TenantID,
						[]string{types.ParseStatusDeleting, types.ParseStatusCancelled}).
					Updates(map[string]any{"parse_status": types.ParseStatusFailed, "error_message": err.Error()})
			}
		}
	}()
	fileSvc, err := p.videoFileService(ctx, tenant, kb, &asset)
	if err != nil {
		return err
	}
	if err = p.setStage(ctx, &run, "probe"); err != nil {
		return err
	}
	path, cleanup, err := p.downloadVideo(ctx, fileSvc, &asset)
	if err != nil {
		return err
	}
	defer cleanup()
	meta, err := media.ProbeFile(ctx, path, media.ProbeLimits{
		MaxBytes: 2 << 30, MaxDurationMS: 8 * 60 * 60 * 1000, MaxPixels: 3840 * 2160,
	})
	if err != nil {
		return err
	}
	result := p.db.WithContext(ctx).Model(&types.VideoAsset{}).
		Where("tenant_id = ? AND knowledge_id = ? AND current_run_id = ?", payload.TenantID, knowledge.ID, run.ID).
		Updates(map[string]any{"duration_ms": meta.DurationMS, "width": meta.Width,
			"height": meta.Height, "video_codec": meta.VideoCodec, "audio_codec": meta.AudioCodec,
			"has_audio": meta.HasAudio, "status": "processing"})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("video run no longer current: %w", asynq.SkipRetry)
	}
	overrides, _ := knowledge.ProcessOverrides()
	effective := ResolveProcessConfig(kb, overrides)
	if meta.HasAudio && effective.ASRConfig.IsASREnabled() {
		if err = p.transcribe(ctx, path, &run, meta.DurationMS, effective.ASRConfig); err != nil {
			return err
		}
	}
	if err = p.analyzeFrames(ctx, fileSvc, path, &run, meta.DurationMS, effective.VLMConfig, effective.VideoConfig); err != nil {
		return err
	}
	if err = p.setStage(ctx, &run, "index"); err != nil {
		return err
	}
	return p.publish(ctx, &asset, &run, knowledge, kb, meta.DurationMS)
}

func (p *VideoProcessor) setStage(ctx context.Context, run *types.VideoRun, stage string) error {
	if err := p.ensureRunCurrent(ctx, run); err != nil {
		return err
	}
	run.Stage, run.Status = stage, "processing"
	result := p.db.WithContext(ctx).Model(&types.VideoRun{}).
		Where("id = ? AND status <> ?", run.ID, "cancelled").
		Updates(map[string]any{"stage": stage, "status": "processing", "failure_code": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("video run cancelled: %w", asynq.SkipRetry)
	}
	return nil
}

func (p *VideoProcessor) ensureRunCurrent(ctx context.Context, run *types.VideoRun) error {
	var state struct {
		CurrentRunID string
		ParseStatus  string
	}
	err := p.db.WithContext(ctx).Table("video_assets AS asset").
		Select("asset.current_run_id, knowledge.parse_status").
		Joins("JOIN knowledges AS knowledge ON knowledge.id = asset.knowledge_id AND knowledge.tenant_id = asset.tenant_id").
		Where("asset.tenant_id = ? AND asset.knowledge_id = ?", run.TenantID, run.KnowledgeID).
		Take(&state).Error
	if err != nil {
		return err
	}
	if state.CurrentRunID != run.ID || state.ParseStatus == types.ParseStatusDeleting ||
		state.ParseStatus == types.ParseStatusCancelled {
		return fmt.Errorf("video run no longer current: %w", asynq.SkipRetry)
	}
	return nil
}

func (p *VideoProcessor) videoFileService(ctx context.Context, tenant *types.Tenant, kb *types.KnowledgeBase, asset *types.VideoAsset) (interfaces.FileService, error) {
	if p.storage == nil {
		return p.files, nil
	}
	backendID := ""
	if kb.StorageBackendID != nil {
		backendID = *kb.StorageBackendID
	}
	if pathBackendID, _, scoped := types.ParseStorageBackendPath(asset.FilePath); scoped {
		backendID = pathBackendID
	}
	fileSvc, _, err := p.storage.ResolveFileService(ctx, tenant, backendID, kb.GetStorageProvider(), strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR")))
	if err != nil {
		return nil, err
	}
	return fileSvc, nil
}

func (p *VideoProcessor) downloadVideo(ctx context.Context, fileSvc interfaces.FileService, asset *types.VideoAsset) (string, func(), error) {
	reader, err := fileSvc.GetFile(ctx, asset.FilePath)
	if err != nil {
		return "", nil, err
	}
	defer reader.Close()
	file, err := os.CreateTemp("", "weknora-video-*.mp4")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.Remove(file.Name()) }
	var size int64
	if size, err = io.Copy(file, io.LimitReader(reader, (2<<30)+1)); err != nil {
		file.Close()
		cleanup()
		return "", nil, err
	}
	if size > 2<<30 {
		file.Close()
		cleanup()
		return "", nil, fmt.Errorf("video exceeds 2 GiB processing limit")
	}
	if err = file.Close(); err != nil {
		cleanup()
		return "", nil, err
	}
	return file.Name(), cleanup, nil
}

func (p *VideoProcessor) transcribe(ctx context.Context, path string, run *types.VideoRun,
	durationMS int64, cfg types.ASRConfig) error {
	if err := p.setStage(ctx, run, "asr"); err != nil {
		return err
	}
	model, err := p.models.GetASRModel(ctx, cfg.ModelID)
	if err != nil {
		return err
	}
	stream, ok := model.(asr.WindowFileTranscriber)
	if !ok {
		return fmt.Errorf("configured ASR model does not support long video windows")
	}
	ctx = asr.WithLanguage(ctx, cfg.Language)
	return stream.TranscribeFileWindows(ctx, path, run.CompletedWindows, func(window asr.Window) error {
		if err := p.ensureRunCurrent(ctx, run); err != nil {
			return err
		}
		start, end := int64(window.Start*1000), int64(window.End*1000)
		if end > durationMS {
			end = durationMS
		}
		err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			progress := tx.Model(&types.VideoRun{}).
				Where("id = ? AND completed_windows = ? AND status <> ?", run.ID, window.Index, "cancelled").
				Updates(map[string]any{"completed_windows": window.Index + 1,
					"total_windows": int((durationMS + 29999) / 30000)})
			if progress.Error != nil {
				return progress.Error
			}
			if progress.RowsAffected != 1 {
				return fmt.Errorf("video window no longer current: %w", asynq.SkipRetry)
			}
			if strings.TrimSpace(window.Text) != "" && end > start {
				segment := types.VideoSegment{ID: uuid.NewString(), TenantID: run.TenantID,
					KnowledgeID: run.KnowledgeID, RunID: run.ID, StartMS: start, EndMS: end}
				if err := tx.Create(&segment).Error; err != nil {
					return err
				}
				evidence := types.VideoEvidence{ID: uuid.NewString(), TenantID: run.TenantID,
					KnowledgeID: run.KnowledgeID, RunID: run.ID, SegmentID: segment.ID,
					Modality: types.VideoModalityASR, StartMS: &start, EndMS: &end,
					Text: strings.TrimSpace(window.Text), ModelVersion: model.GetModelName(), ConfigJSON: "{}"}
				if err := tx.Create(&evidence).Error; err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			run.CompletedWindows = window.Index + 1
		}
		return err
	})
}

func (p *VideoProcessor) analyzeFrames(ctx context.Context, fileSvc interfaces.FileService, path string, run *types.VideoRun,
	durationMS int64, config types.VLMConfig, videoConfig types.VideoProcessConfig) error {
	if err := p.setStage(ctx, run, "frames"); err != nil {
		return err
	}
	var vision vlm.VLM
	if config.IsEnabled() && config.ModelID != "" {
		var err error
		vision, err = p.models.GetVLMModel(ctx, config.ModelID)
		if err != nil {
			logger.Warnf(ctx, "Video visual model unavailable for run %s: %v", run.ID, err)
		}
	}
	step := int64(videoConfig.FrameIntervalSeconds) * 1000
	if (durationMS+step-1)/step > int64(videoConfig.MaxFrames) {
		step = (durationMS + int64(videoConfig.MaxFrames) - 1) / int64(videoConfig.MaxFrames)
	}
	for frameMS := int64(0); frameMS < durationMS; frameMS += step {
		if err := p.ensureRunCurrent(ctx, run); err != nil {
			return err
		}
		var count int64
		if err := p.db.WithContext(ctx).Model(&types.VideoKeyframe{}).
			Where("run_id = ? AND frame_ms = ?", run.ID, frameMS).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		jpeg, err := extractVideoFrame(ctx, path, frameMS)
		if err != nil {
			return err
		}
		ocrText, err := ocrVideoFrame(ctx, jpeg, videoConfig.OCRLanguage)
		if err != nil {
			return fmt.Errorf("OCR frame at %d ms: %w", frameMS, err)
		}
		var caption string
		if vision != nil {
			caption, err = vision.Predict(ctx, [][]byte{jpeg},
				"Describe only visible facts, text, diagrams and actions in this video frame. Be concise. Do not infer events outside this frame.")
			if err != nil {
				logger.Warnf(ctx, "Video visual description failed for run %s at %d ms: %v", run.ID, frameMS, err)
				caption = ""
			}
		}
		framePath, err := fileSvc.SaveBytes(ctx, jpeg, run.TenantID,
			"video-"+run.KnowledgeID+"-"+strconv.FormatInt(frameMS, 10)+".jpg", false)
		if err != nil {
			return err
		}
		end := frameMS + 1
		if end > durationMS {
			end = durationMS
		}
		if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			frameSegmentID := ""
			for _, item := range []struct{ modality, text, model string }{
				{types.VideoModalityOCR, ocrText, "tesseract"},
				{types.VideoModalityVLM, caption, config.ModelID},
			} {
				if strings.TrimSpace(item.text) == "" {
					continue
				}
				segment := types.VideoSegment{ID: uuid.NewString(), TenantID: run.TenantID,
					KnowledgeID: run.KnowledgeID, RunID: run.ID, StartMS: frameMS, EndMS: end}
				if err := tx.Create(&segment).Error; err != nil {
					return err
				}
				if frameSegmentID == "" {
					frameSegmentID = segment.ID
				}
				evidence := types.VideoEvidence{ID: uuid.NewString(), TenantID: run.TenantID,
					KnowledgeID: run.KnowledgeID, RunID: run.ID, SegmentID: segment.ID,
					Modality: item.modality, FrameMS: &frameMS, Text: strings.TrimSpace(item.text),
					ModelVersion: item.model, ConfigJSON: "{}"}
				if err := tx.Create(&evidence).Error; err != nil {
					return err
				}
			}
			if frameSegmentID == "" {
				segment := types.VideoSegment{ID: uuid.NewString(), TenantID: run.TenantID,
					KnowledgeID: run.KnowledgeID, RunID: run.ID, StartMS: frameMS, EndMS: end}
				if err := tx.Create(&segment).Error; err != nil {
					return err
				}
				frameSegmentID = segment.ID
			}
			keyframe := types.VideoKeyframe{ID: uuid.NewString(), TenantID: run.TenantID,
				KnowledgeID: run.KnowledgeID, RunID: run.ID, SegmentID: frameSegmentID,
				FrameMS: frameMS, FilePath: framePath}
			return tx.Create(&keyframe).Error
		}); err != nil {
			_ = fileSvc.DeleteFile(ctx, framePath)
			return err
		}
	}
	return nil
}

func extractVideoFrame(ctx context.Context, path string, frameMS int64) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-ss",
		fmt.Sprintf("%.3f", float64(frameMS)/1000), "-i", path,
		"-frames:v", "1", "-vf", "scale='min(1280,iw)':-2", "-f", "image2pipe", "-vcodec", "mjpeg", "pipe:1")
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("extract frame at %d ms: %w", frameMS, err)
	}
	return data, nil
}

func ocrVideoFrame(ctx context.Context, jpeg []byte, language string) (string, error) {
	file, err := os.CreateTemp("", "weknora-frame-*.jpg")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(jpeg); err != nil {
		file.Close()
		return "", err
	}
	file.Close()
	cmd := exec.CommandContext(ctx, "tesseract", filepath.Clean(file.Name()), "stdout", "-l", language)
	output, err := cmd.Output()
	return strings.TrimSpace(string(output)), err
}

func (p *VideoProcessor) publish(ctx context.Context, asset *types.VideoAsset, run *types.VideoRun,
	knowledge *types.Knowledge, kb *types.KnowledgeBase, durationMS int64) error {
	if err := p.ensureRunCurrent(ctx, run); err != nil {
		return err
	}
	var segments []types.VideoSegment
	if err := p.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ?",
		run.TenantID, run.KnowledgeID, run.ID).Order("start_ms, id").Find(&segments).Error; err != nil {
		return err
	}
	chunks := make([]types.ParsedChunk, 0, len(segments))
	indexed := make([]string, 0, len(segments))
	charOffset := 0
	for _, segment := range segments {
		var evidence []types.VideoEvidence
		if err := p.db.WithContext(ctx).Where("segment_id = ? AND run_id = ?", segment.ID, run.ID).
			Order("id").Find(&evidence).Error; err != nil {
			return err
		}
		parts := make([]string, 0, len(evidence))
		for _, item := range evidence {
			parts = append(parts, item.Modality+": "+item.Text)
		}
		content := strings.Join(parts, "\n")
		if content == "" {
			continue
		}
		length := len([]rune(content))
		chunks = append(chunks, types.ParsedChunk{Content: content, Seq: len(chunks),
			Start: charOffset, End: charOffset + length})
		indexed = append(indexed, segment.ID)
		charOffset += length
	}
	if len(chunks) == 0 {
		return fmt.Errorf("video contains no searchable audio or visual evidence")
	}
	if asset.CurrentRunID != run.ID || durationMS <= 0 {
		return fmt.Errorf("video run superseded: %w", asynq.SkipRetry)
	}
	alreadySwitched := asset.ActiveRunID == run.ID
	if !alreadySwitched {
		indexer, ok := p.indexer.(interface {
			IndexVideoReplacement(context.Context, *types.KnowledgeBase, *types.Knowledge, []types.ParsedChunk, []string) error
		})
		if !ok {
			return fmt.Errorf("knowledge indexer does not support video replacement")
		}
		if err := indexer.IndexVideoReplacement(ctx, kb, knowledge, chunks, indexed); err != nil {
			return err
		}
	}
	if alreadySwitched {
		for i := range chunks {
			chunks[i].ChunkID = indexed[i]
		}
	}
	oldIDs := []string{}
	if asset.ActiveRunID != "" {
		if err := p.db.WithContext(ctx).Model(&types.VideoSegment{}).
			Where("tenant_id = ? AND knowledge_id = ? AND run_id <> ? AND chunk_id <> ''", run.TenantID, run.KnowledgeID, run.ID).
			Pluck("chunk_id", &oldIDs).Error; err != nil {
			return err
		}
	}
	if !alreadySwitched {
		if err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i, segmentID := range indexed {
				chunkID := chunks[i].ChunkID
				if err := tx.Model(&types.VideoSegment{}).Where("id = ? AND run_id = ?", segmentID, run.ID).
					Update("chunk_id", chunkID).Error; err != nil {
					return err
				}
			}
			now := time.Now()
			if err := tx.Model(&types.Chunk{}).Where("id IN ?", indexed).Update("is_enabled", true).Error; err != nil {
				return err
			}
			if err := tx.Model(&types.Knowledge{}).Where("id = ? AND tenant_id = ?", knowledge.ID, knowledge.TenantID).
				Updates(map[string]any{"parse_status": types.ParseStatusProcessing, "enable_status": "enabled", "error_message": "", "processed_at": now}).Error; err != nil {
				return err
			}
			result := tx.Model(&types.VideoAsset{}).Where("knowledge_id = ? AND current_run_id = ?",
				knowledge.ID, run.ID).Updates(map[string]any{"active_run_id": run.ID,
				"status": "completed", "failure_code": "", "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("video run superseded: %w", asynq.SkipRetry)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	indexer, ok := p.indexer.(interface {
		FinalizeVideoReplacement(context.Context, *types.KnowledgeBase, *types.Knowledge, []string, []string) error
	})
	if !ok {
		return fmt.Errorf("knowledge indexer cannot finalize video replacement")
	}
	if err := indexer.FinalizeVideoReplacement(ctx, kb, knowledge, indexed, oldIDs); err != nil {
		return err
	}
	return p.db.WithContext(ctx).Model(&types.VideoRun{}).Where("id = ?", run.ID).
		Updates(map[string]any{"status": "completed", "stage": "completed", "published_at": time.Now(),
			"processor_version": videoProcessorVersion, "failure_code": ""}).Error
}

func (s *knowledgeService) IndexVideoReplacement(ctx context.Context, kb *types.KnowledgeBase,
	knowledge *types.Knowledge, chunks []types.ParsedChunk, segmentIDs []string) error {
	if s.db == nil || len(chunks) != len(segmentIDs) {
		return fmt.Errorf("invalid video replacement chunks")
	}
	now := time.Now()
	for i, segmentID := range segmentIDs {
		chunk := types.Chunk{ID: segmentID, TenantID: knowledge.TenantID,
			KnowledgeID: knowledge.ID, KnowledgeBaseID: kb.ID, Content: chunks[i].Content,
			ChunkIndex: i, IsEnabled: false, IndexStatus: "ready", ChunkType: types.ChunkTypeText,
			StartAt: int(chunks[i].Start), EndAt: int(chunks[i].End), CreatedAt: now, UpdatedAt: now}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&chunk).Error; err != nil {
				return err
			}
			return tx.Model(&types.Chunk{}).Where("id = ?", segmentID).Update("is_enabled", false).Error
		}); err != nil {
			return err
		}
		chunks[i].ChunkID = segmentID
	}
	if !kb.NeedsEmbeddingModel() {
		return nil
	}
	model, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
	if err != nil {
		return err
	}
	engine, err := retriever.CreateRetrieveEngineForKB(ctx, s.retrieveEngine, s.ownership,
		knowledge.TenantID, kb.VectorStoreID)
	if err != nil {
		return err
	}
	for start := 0; start < len(chunks); start += 200 {
		end := start + 200
		if end > len(chunks) {
			end = len(chunks)
		}
		infos := make([]*types.IndexInfo, 0, end-start)
		for i := start; i < end; i++ {
			infos = append(infos, &types.IndexInfo{Content: buildKnowledgeIndexContent(knowledge, chunks[i].Content),
				SourceID: segmentIDs[i], SourceType: types.ChunkSourceType, ChunkID: segmentIDs[i],
				KnowledgeID: knowledge.ID, KnowledgeBaseID: kb.ID, IsEnabled: false})
		}
		if err := engine.BatchIndex(ctx, model, infos); err != nil {
			return err
		}
	}
	return nil
}

func (s *knowledgeService) FinalizeVideoReplacement(ctx context.Context, kb *types.KnowledgeBase,
	knowledge *types.Knowledge, newIDs, oldIDs []string) error {
	if kb.NeedsEmbeddingModel() {
		model, err := s.modelService.GetEmbeddingModel(ctx, kb.EmbeddingModelID)
		if err != nil {
			return err
		}
		engine, err := retriever.CreateRetrieveEngineForKB(ctx, s.retrieveEngine, s.ownership,
			knowledge.TenantID, kb.VectorStoreID)
		if err != nil {
			return err
		}
		status := make(map[string]bool, len(newIDs)+len(oldIDs))
		for _, id := range oldIDs {
			status[id] = false
		}
		for _, id := range newIDs {
			status[id] = true
		}
		if err := engine.BatchUpdateChunkEnabledStatus(ctx, status); err != nil {
			return err
		}
		if len(oldIDs) > 0 {
			if err := engine.DeleteByChunkIDList(ctx, oldIDs, model.GetDimensions(), knowledge.Type); err != nil {
				return err
			}
		}
	}
	if len(oldIDs) > 0 {
		if err := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND id IN ?", knowledge.TenantID, knowledge.ID, oldIDs).
			Delete(&types.Chunk{}).Error; err != nil {
			return err
		}
	}
	if s.graphEngine != nil {
		if err := s.graphEngine.DelGraph(ctx, []types.NameSpace{{KnowledgeBase: kb.ID, Knowledge: knowledge.ID}}); err != nil {
			return err
		}
	}
	s.prepareWikiForReparse(ctx, knowledge)
	return s.enqueueKnowledgePostProcessTask(ctx, knowledge)
}
