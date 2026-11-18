package handler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/application/service"
	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/filetransport"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/Tencent/WeKnora/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

const videoPartSize int64 = 8 << 20
const maxVideoSize int64 = 2 << 30
const videoPlaybackTTL = 5 * time.Minute

type videoUploadRequest struct {
	FileName      string                           `json:"file_name" binding:"required"`
	Size          int64                            `json:"size" binding:"required"`
	MIME          string                           `json:"mime" binding:"required"`
	FolderPath    string                           `json:"folder_path"`
	TagIDs        []string                         `json:"tag_ids"`
	ProcessConfig *types.KnowledgeProcessOverrides `json:"process_config"`
}

type videoUploadResponse struct {
	UploadID       string `json:"upload_id"`
	PartSize       int64  `json:"part_size"`
	CompletedParts []int  `json:"completed_parts"`
	Status         string `json:"status"`
	KnowledgeID    string `json:"knowledge_id,omitempty"`
}

func videoUploadData(upload *types.VideoUpload, parts []types.VideoUploadPart) videoUploadResponse {
	completed := make([]int, 0, len(parts))
	for _, part := range parts {
		completed = append(completed, part.PartNumber)
	}
	sort.Ints(completed)
	return videoUploadResponse{UploadID: upload.ID, PartSize: videoPartSize, CompletedParts: completed, Status: upload.Status, KnowledgeID: upload.KnowledgeID}
}

func videoError(c *gin.Context, err error) { _ = c.Error(err) }

func (h *KnowledgeHandler) videoUploadScope(c *gin.Context) (*types.KnowledgeBase, uint64, context.Context, error) {
	kb, _, tenantID, permission, err := h.validateKnowledgeBaseAccess(c)
	if err != nil {
		return nil, 0, nil, err
	}
	if permission != types.OrgRoleAdmin && permission != types.OrgRoleEditor {
		return nil, 0, nil, werrors.NewForbiddenError("No permission to upload video")
	}
	return kb, tenantID, types.WithExecutionTenant(c.Request.Context(), tenantID), nil
}

func (h *KnowledgeHandler) CreateVideoUpload(c *gin.Context) {
	if os.Getenv("VIDEO_INGEST_ENABLED") != "1" {
		videoError(c, werrors.NewForbiddenError("Video ingestion is disabled"))
		return
	}
	kb, tenantID, ctx, err := h.videoUploadScope(c)
	if err != nil {
		videoError(c, err)
		return
	}
	if kb.Type == types.KnowledgeBaseTypeFAQ {
		videoError(c, werrors.NewBadRequestError("Knowledge base cannot accept video"))
		return
	}
	var req videoUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		videoError(c, werrors.NewBadRequestError("Invalid video upload request"))
		return
	}
	name, err := utils.SafeFileName(filepath.Base(req.FileName))
	mediaType, _, mimeErr := mime.ParseMediaType(req.MIME)
	if err != nil || mimeErr != nil || name == "." || name == "" || !strings.EqualFold(filepath.Ext(name), ".mp4") || req.Size <= 0 || req.Size > maxVideoSize || !strings.EqualFold(mediaType, "video/mp4") {
		videoError(c, werrors.NewBadRequestError("Invalid video name, size, or MIME type"))
		return
	}
	req.MIME = strings.ToLower(mediaType)
	folder := types.NormalizeKnowledgeFolderPath(req.FolderPath)
	if len(folder) > 1024 {
		videoError(c, werrors.NewBadRequestError("Folder path too long"))
		return
	}
	if _, valid := utils.ValidateInput(folder); !valid && folder != "" {
		videoError(c, werrors.NewBadRequestError("Invalid folder path"))
		return
	}
	unique := make(map[string]struct{}, len(req.TagIDs))
	tags := make([]string, 0, len(req.TagIDs))
	for _, tag := range req.TagIDs {
		if tag != "" {
			if _, seen := unique[tag]; !seen {
				unique[tag] = struct{}{}
				tags = append(tags, tag)
			}
		}
	}
	if len(tags) > 50 {
		videoError(c, werrors.NewBadRequestError("Too many tags"))
		return
	}
	if req.ProcessConfig != nil {
		if err := service.ValidateProcessOverrides(ctx, kb, req.ProcessConfig, []string{"mp4"}); err != nil {
			videoError(c, err)
			return
		}
	}
	if err := h.checkVideoQuota(ctx, tenantID, req.Size); err != nil {
		videoError(c, err)
		return
	}
	tagJSON, _ := json.Marshal(tags)
	configJSON := ""
	if req.ProcessConfig != nil {
		encoded, err := json.Marshal(req.ProcessConfig)
		if err != nil {
			videoError(c, werrors.NewBadRequestError("Invalid video process configuration"))
			return
		}
		configJSON = string(encoded)
	}
	upload := &types.VideoUpload{ID: uuid.NewString(), TenantID: tenantID, KnowledgeBaseID: c.Param("id"), FileName: name, Size: req.Size, MIME: req.MIME, FolderPath: folder, TagIDsJSON: string(tagJSON), ProcessConfigJSON: configJSON, Status: "uploading", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := h.videoRepo.CreateUpload(ctx, upload); err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot create video upload"))
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": videoUploadData(upload, nil)})
}

func (h *KnowledgeHandler) checkVideoQuota(ctx context.Context, tenantID uint64, size int64) error {
	tenant, err := h.videoTenants.GetTenantByID(ctx, tenantID)
	if err != nil || tenant == nil {
		return werrors.NewInternalServerError("Cannot check workspace storage quota")
	}
	if tenant.StorageQuota > 0 && (size > tenant.StorageQuota || tenant.StorageUsed > tenant.StorageQuota-size) {
		return werrors.NewBadRequestError("Workspace storage quota exceeded")
	}
	return nil
}

func (h *KnowledgeHandler) loadVideoUpload(c *gin.Context) (*types.VideoUpload, []types.VideoUploadPart, context.Context, error) {
	_, tenantID, ctx, err := h.videoUploadScope(c)
	if err != nil {
		return nil, nil, nil, err
	}
	upload, parts, err := h.videoRepo.GetUpload(ctx, tenantID, c.Param("id"), c.Param("upload_id"))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, werrors.NewNotFoundError("Video upload not found")
	}
	return upload, parts, ctx, err
}

func (h *KnowledgeHandler) GetVideoUpload(c *gin.Context) {
	upload, parts, _, err := h.loadVideoUpload(c)
	if err != nil {
		videoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": videoUploadData(upload, parts)})
}

func (h *KnowledgeHandler) PutVideoUploadPart(c *gin.Context) {
	upload, _, ctx, err := h.loadVideoUpload(c)
	if err != nil {
		videoError(c, err)
		return
	}
	if upload.Status != "uploading" {
		videoError(c, werrors.NewBadRequestError("Upload is not writable"))
		return
	}
	number, err := strconv.Atoi(c.Param("part_number"))
	partCount := int((upload.Size + videoPartSize - 1) / videoPartSize)
	if err != nil || number < 1 || number > partCount {
		videoError(c, werrors.NewBadRequestError("Invalid part number"))
		return
	}
	want := videoPartSize
	if number == partCount {
		want = upload.Size - int64(number-1)*videoPartSize
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, want+1)
	bytes, err := io.ReadAll(c.Request.Body)
	if err != nil || int64(len(bytes)) != want {
		videoError(c, werrors.NewBadRequestError("Part size does not match upload"))
		return
	}
	path, err := h.videoFiles.SaveBytes(ctx, bytes, upload.TenantID, uuid.NewString()+".part", true)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot store video part"))
		return
	}
	previous, err := h.videoRepo.PutPart(ctx, upload.ID, &types.VideoUploadPart{PartNumber: number, Size: int64(len(bytes)), FilePath: path})
	if err != nil {
		_ = h.videoFiles.DeleteFile(ctx, path)
		videoError(c, werrors.NewBadRequestError("Upload is not writable"))
		return
	}
	if previous != "" {
		_ = h.videoFiles.DeleteFile(ctx, previous)
	}
	upload, parts, err := h.videoRepo.GetUpload(ctx, upload.TenantID, upload.KnowledgeBaseID, upload.ID)
	if err != nil {
		videoError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": videoUploadData(upload, parts)})
}

func (h *KnowledgeHandler) DeleteVideoUpload(c *gin.Context) {
	upload, parts, ctx, err := h.loadVideoUpload(c)
	if err != nil {
		videoError(c, err)
		return
	}
	if upload.Status != "uploading" {
		videoError(c, werrors.NewBadRequestError("Upload cannot be deleted"))
		return
	}
	if err := h.videoRepo.DeleteUpload(ctx, upload.TenantID, upload.KnowledgeBaseID, upload.ID); err != nil {
		videoError(c, err)
		return
	}
	for _, part := range parts {
		_ = h.videoFiles.DeleteFile(ctx, part.FilePath)
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": videoUploadResponse{UploadID: upload.ID, PartSize: videoPartSize, CompletedParts: []int{}, Status: "deleted"}})
}

func (h *KnowledgeHandler) videoFinalStorage(ctx context.Context, kb *types.KnowledgeBase, tenantID uint64, filePath ...string) (interfaces.FileService, error) {
	tenant, err := h.videoTenants.GetTenantByID(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if h.videoStorage == nil {
		return h.videoFiles, nil
	}
	backendID := ""
	if kb.StorageBackendID != nil {
		backendID = *kb.StorageBackendID
	}
	if len(filePath) > 0 {
		if pathBackendID, _, scoped := types.ParseStorageBackendPath(filePath[0]); scoped {
			backendID = pathBackendID
		}
	}
	svc, _, err := h.videoStorage.ResolveFileService(ctx, tenant, backendID, kb.GetStorageProvider(), strings.TrimSpace(os.Getenv("LOCAL_STORAGE_BASE_DIR")))
	if err != nil {
		return nil, err
	}
	if svc == nil {
		return h.videoFiles, nil
	}
	return svc, nil
}

// assembleVideoUpload uses the standard multipart parser to provide the storage
// adapters with a FileHeader while keeping the full source off the Go heap.
func (h *KnowledgeHandler) assembleVideoUpload(ctx context.Context, upload *types.VideoUpload, parts []types.VideoUploadPart) (*multipart.Form, *multipart.FileHeader, string, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	result := make(chan struct {
		hash string
		err  error
	}, 1)
	go func() {
		var sum = sha256.New()
		field, err := writer.CreateFormFile("file", upload.FileName)
		if err == nil {
			for _, part := range parts {
				var src io.ReadCloser
				src, err = h.videoFiles.GetFile(ctx, part.FilePath)
				if err != nil {
					break
				}
				var copied int64
				copied, err = io.Copy(io.MultiWriter(field, sum), src)
				_ = src.Close()
				if err != nil || copied != part.Size {
					if err == nil {
						err = fmt.Errorf("video part size changed")
					}
					break
				}
			}
		}
		if err == nil {
			err = writer.Close()
		}
		_ = pw.CloseWithError(err)
		result <- struct {
			hash string
			err  error
		}{hash: hex.EncodeToString(sum.Sum(nil)), err: err}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/", pr)
	if err != nil {
		_ = pr.Close()
		return nil, nil, "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	err = req.ParseMultipartForm(0)
	if err != nil {
		_ = pr.Close()
	}
	out := <-result
	if err != nil {
		return nil, nil, "", err
	}
	if out.err != nil {
		if req.MultipartForm != nil {
			_ = req.MultipartForm.RemoveAll()
		}
		return nil, nil, "", out.err
	}
	files := req.MultipartForm.File["file"]
	if len(files) != 1 {
		_ = req.MultipartForm.RemoveAll()
		return nil, nil, "", fmt.Errorf("assembled video is missing")
	}
	return req.MultipartForm, files[0], out.hash, nil
}

func (h *KnowledgeHandler) CompleteVideoUpload(c *gin.Context) {
	upload, parts, ctx, err := h.loadVideoUpload(c)
	if err != nil {
		videoError(c, err)
		return
	}
	if upload.Status == "completed" {
		run, err := h.videoRepo.GetCurrentRun(ctx, upload.TenantID, upload.KnowledgeID)
		if err != nil {
			videoError(c, werrors.NewInternalServerError("Cannot read video processing run"))
			return
		}
		if run.Status == "queued" {
			if err := h.enqueueVideoRun(upload.TenantID, upload.KnowledgeID, run.ID); err != nil {
				videoError(c, werrors.NewInternalServerError("Video saved but processing could not be queued"))
				return
			}
		}
		c.JSON(http.StatusOK, gin.H{"success": true, "data": videoUploadData(upload, parts)})
		return
	}
	if upload.Status != "uploading" && upload.Status != "completing" {
		videoError(c, werrors.NewBadRequestError("Upload is already completing"))
		return
	}
	count := int((upload.Size + videoPartSize - 1) / videoPartSize)
	if len(parts) != count {
		videoError(c, werrors.NewBadRequestError("Video upload has missing parts"))
		return
	}
	for i, part := range parts {
		want := videoPartSize
		if i == count-1 {
			want = upload.Size - int64(i)*videoPartSize
		}
		if part.PartNumber != i+1 || part.Size != want {
			videoError(c, werrors.NewBadRequestError("Video upload has invalid parts"))
			return
		}
	}
	claimed, err := h.videoRepo.ClaimUpload(ctx, upload.TenantID, upload.KnowledgeBaseID, upload.ID)
	if err != nil || !claimed {
		videoError(c, werrors.NewBadRequestError("Upload is already completing"))
		return
	}
	defer func() { _ = h.videoRepo.ResetUpload(context.WithoutCancel(ctx), upload.ID) }()
	upload, parts, err = h.videoRepo.GetUpload(ctx, upload.TenantID, upload.KnowledgeBaseID, upload.ID)
	if err != nil || len(parts) != count {
		videoError(c, werrors.NewBadRequestError("Video upload changed during completion"))
		return
	}
	for i, part := range parts {
		want := videoPartSize
		if i == count-1 {
			want = upload.Size - int64(i)*videoPartSize
		}
		if part.PartNumber != i+1 || part.Size != want {
			videoError(c, werrors.NewBadRequestError("Video upload changed during completion"))
			return
		}
	}
	form, file, hash, err := h.assembleVideoUpload(ctx, upload, parts)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot assemble video"))
		return
	}
	defer form.RemoveAll()
	kb, err := h.kbService.GetKnowledgeBaseByID(ctx, upload.KnowledgeBaseID)
	if err != nil || kb == nil || kb.Type == types.KnowledgeBaseTypeFAQ {
		videoError(c, werrors.NewBadRequestError("Knowledge base cannot accept video"))
		return
	}
	if err := h.checkVideoQuota(ctx, upload.TenantID, upload.Size); err != nil {
		videoError(c, err)
		return
	}
	storage, err := h.videoFinalStorage(ctx, kb, upload.TenantID)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Video storage unavailable"))
		return
	}
	knowledgeID := uuid.NewString()
	path, err := storage.SaveFile(ctx, file, upload.TenantID, knowledgeID)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot store video"))
		return
	}
	now := time.Now()
	runID := uuid.NewString()
	knowledge := &types.Knowledge{ID: knowledgeID, TenantID: upload.TenantID, KnowledgeBaseID: upload.KnowledgeBaseID, Type: "file", Channel: "web", Title: upload.FileName, FileName: upload.FileName, FolderPath: upload.FolderPath, FileType: strings.TrimPrefix(strings.ToLower(filepath.Ext(upload.FileName)), "."), FileSize: upload.Size, StorageSize: upload.Size, FileHash: hash, FilePath: path, ParseStatus: "pending", EnableStatus: "disabled", EmbeddingModelID: kb.EmbeddingModelID, CreatedAt: now, UpdatedAt: now}
	if upload.ProcessConfigJSON != "" {
		var config types.KnowledgeProcessOverrides
		if err := json.Unmarshal([]byte(upload.ProcessConfigJSON), &config); err != nil {
			_ = storage.DeleteFile(ctx, path)
			videoError(c, werrors.NewInternalServerError("Stored video process configuration is invalid"))
			return
		}
		if err := knowledge.SetProcessOverrides(&config); err != nil {
			_ = storage.DeleteFile(ctx, path)
			videoError(c, werrors.NewInternalServerError("Cannot save video process configuration"))
			return
		}
	}
	asset := &types.VideoAsset{KnowledgeID: knowledgeID, TenantID: upload.TenantID, FilePath: path, MIME: upload.MIME, Status: "queued", CurrentRunID: runID, CreatedAt: now, UpdatedAt: now}
	run := &types.VideoRun{ID: runID, TenantID: upload.TenantID, KnowledgeID: knowledgeID, ProcessorVersion: "video-v1", Status: "queued", Stage: "queued", CreatedAt: now}
	var tags []string
	if err := json.Unmarshal([]byte(upload.TagIDsJSON), &tags); err != nil {
		_ = storage.DeleteFile(ctx, path)
		videoError(c, werrors.NewInternalServerError("Invalid upload tags"))
		return
	}
	if err := h.videoRepo.CompleteUpload(ctx, upload, knowledge, asset, run, tags); err != nil {
		_ = storage.DeleteFile(ctx, path)
		videoError(c, werrors.NewInternalServerError("Cannot complete video upload"))
		return
	}
	for _, part := range parts {
		_ = h.videoFiles.DeleteFile(ctx, part.FilePath)
	}
	err = h.enqueueVideoRun(upload.TenantID, knowledgeID, runID)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Video saved but processing could not be queued"))
		return
	}
	upload.Status, upload.KnowledgeID = "completed", knowledgeID
	c.JSON(http.StatusOK, gin.H{"success": true, "data": videoUploadData(upload, parts)})
}

func (h *KnowledgeHandler) enqueueVideoRun(tenantID uint64, knowledgeID, runID string) error {
	payload, err := json.Marshal(map[string]interface{}{"tenant_id": tenantID, "knowledge_id": knowledgeID, "processing_run_id": runID})
	if err != nil {
		return err
	}
	_, err = h.asynqClient.Enqueue(asynq.NewTask(types.TypeVideoProcess, payload,
		asynq.TaskID(runID), asynq.Queue(types.QueueMedia), asynq.MaxRetry(3), asynq.Timeout(4*time.Hour)))
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (h *KnowledgeHandler) GetVideo(c *gin.Context) {
	knowledge, _, err := h.resolveKnowledgeAndValidateKBAccess(c, c.Param("id"), types.OrgRoleViewer)
	if err != nil {
		videoError(c, err)
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		videoError(c, werrors.NewBadRequestError("Invalid video page offset"))
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "100"))
	if err != nil || limit < 1 || limit > 100 {
		videoError(c, werrors.NewBadRequestError("Video page limit must be between 1 and 100"))
		return
	}
	segmentID := c.Query("segment_id")
	if len(segmentID) > 64 {
		videoError(c, werrors.NewBadRequestError("Invalid video segment ID"))
		return
	}
	nextOffset := 0
	asset, run, segments, evidences, keyframes, err := h.videoRepo.GetVideoPage(
		c.Request.Context(), knowledge.TenantID, knowledge.ID, offset, limit, segmentID, &nextOffset)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		videoError(c, werrors.NewNotFoundError("Video not found"))
		return
	}
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot read video"))
		return
	}
	var next *int
	if nextOffset > 0 {
		next = &nextOffset
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"asset": asset, "run": run,
		"segments": segments, "evidences": evidences, "keyframes": keyframes, "next_offset": next}})
}

func (h *KnowledgeHandler) GetVideoStatus(c *gin.Context) {
	knowledge, _, err := h.resolveKnowledgeAndValidateKBAccess(c, c.Param("id"), types.OrgRoleViewer)
	if err != nil {
		videoError(c, err)
		return
	}
	asset, run, err := h.videoRepo.GetVideoStatus(c.Request.Context(), knowledge.TenantID, knowledge.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		videoError(c, werrors.NewNotFoundError("Video not found"))
		return
	}
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot read video status"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"upload_status": "completed",
		"process_status": knowledge.ParseStatus, "asset": asset, "run": run}})
}

type videoPlaybackGrant struct {
	KnowledgeID   string    `json:"knowledge_id"`
	UserID        string    `json:"user_id"`
	TenantID      uint64    `json:"tenant_id"`
	Authorization string    `json:"authorization"`
	ExpiresAt     time.Time `json:"expires_at"`
}

func (h *KnowledgeHandler) CreateVideoPlaybackToken(c *gin.Context) {
	knowledge, _, err := h.resolveKnowledgeAndValidateKBAccess(c, c.Param("id"), types.OrgRoleViewer)
	if err != nil {
		videoError(c, err)
		return
	}
	userID := c.GetString(types.UserIDContextKey.String())
	auth := c.GetHeader("Authorization")
	if userID == "" || !strings.HasPrefix(auth, "Bearer ") {
		videoError(c, werrors.NewForbiddenError("Bearer session required for video playback"))
		return
	}
	if _, err := h.videoRepo.GetAsset(c.Request.Context(), knowledge.TenantID, knowledge.ID); err != nil {
		videoError(c, werrors.NewNotFoundError("Video not found"))
		return
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		videoError(c, err)
		return
	}
	token := hex.EncodeToString(bytes)
	grant := videoPlaybackGrant{KnowledgeID: knowledge.ID, UserID: userID, TenantID: c.GetUint64(types.TenantIDContextKey.String()), Authorization: auth, ExpiresAt: time.Now().Add(videoPlaybackTTL)}
	encoded, _ := json.Marshal(grant)
	if h.videoRedis != nil {
		encoded, err = service.EncryptVideoPlaybackGrant(encoded)
		if err != nil {
			videoError(c, werrors.NewInternalServerError("Cannot create playback token"))
			return
		}
		err = h.videoRedis.Set(c.Request.Context(), "video:playback:"+token, encoded, videoPlaybackTTL).Err()
	} else {
		h.videoTokens.Store(token, grant)
		time.AfterFunc(videoPlaybackTTL, func() { h.videoTokens.Delete(token) })
	}
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot create playback token"))
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"token": token, "expires_at": grant.ExpiresAt}})
}

// InjectVideoPlaybackAuth runs before the normal Auth middleware for native
// video requests that cannot attach headers. The normal middleware still
// validates the original bearer and current tenant membership on every Range.
func (h *KnowledgeHandler) InjectVideoPlaybackAuth(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.Next()
		return
	}
	var grant videoPlaybackGrant
	if h.videoRedis != nil {
		encoded, err := h.videoRedis.Get(c.Request.Context(), "video:playback:"+token).Bytes()
		if err != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		plain, err := service.DecryptVideoPlaybackGrant(encoded)
		if err != nil || json.Unmarshal(plain, &grant) != nil {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
	} else {
		value, ok := h.videoTokens.Load(token)
		if !ok {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		grant = value.(videoPlaybackGrant)
	}
	if grant.KnowledgeID != c.Param("id") || time.Now().After(grant.ExpiresAt) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	c.Request.Header.Set("Authorization", grant.Authorization)
	c.Request.Header.Set("X-Tenant-ID", strconv.FormatUint(grant.TenantID, 10))
	c.Set("video_playback_user_id", grant.UserID)
	c.Next()
}

func (h *KnowledgeHandler) StreamVideo(c *gin.Context) {
	if c.GetHeader("X-API-Key") != "" || !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if bound := c.GetString("video_playback_user_id"); bound != "" && bound != c.GetString(types.UserIDContextKey.String()) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, c.Param("id"), types.OrgRoleViewer)
	if err != nil {
		videoError(c, err)
		return
	}
	asset, err := h.videoRepo.GetAsset(effCtx, knowledge.TenantID, knowledge.ID)
	if err != nil {
		videoError(c, werrors.NewNotFoundError("Video not found"))
		return
	}
	file, _, err := h.kgService.GetKnowledgeFile(effCtx, knowledge.ID)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Cannot read video source"))
		return
	}
	if err := serveVideoRange(c.Writer, c.Request, file, knowledge.FileName, asset.MIME, knowledge.FileSize); err != nil {
		_ = c.Error(err)
	}
}

func (h *KnowledgeHandler) StreamVideoKeyframe(c *gin.Context) {
	if c.GetHeader("X-API-Key") != "" || !strings.HasPrefix(c.GetHeader("Authorization"), "Bearer ") {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if bound := c.GetString("video_playback_user_id"); bound != "" && bound != c.GetString(types.UserIDContextKey.String()) {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	knowledge, effCtx, err := h.resolveKnowledgeAndValidateKBAccess(c, c.Param("id"), types.OrgRoleViewer)
	if err != nil {
		videoError(c, err)
		return
	}
	frame, err := h.videoRepo.GetActiveKeyframe(effCtx, knowledge.TenantID, knowledge.ID, c.Param("frame_id"))
	if err != nil {
		videoError(c, werrors.NewNotFoundError("Video keyframe not found"))
		return
	}
	kb, err := h.kbService.GetKnowledgeBaseByID(effCtx, knowledge.KnowledgeBaseID)
	if err != nil || kb == nil {
		videoError(c, werrors.NewNotFoundError("Knowledge base not found"))
		return
	}
	storage, err := h.videoFinalStorage(effCtx, kb, knowledge.TenantID, frame.FilePath)
	if err != nil {
		videoError(c, werrors.NewInternalServerError("Video storage unavailable"))
		return
	}
	reader, err := storage.GetFile(effCtx, frame.FilePath)
	if err != nil {
		videoError(c, werrors.NewNotFoundError("Video keyframe not found"))
		return
	}
	defer reader.Close()
	const maxFrameBytes = 5 << 20
	data, err := io.ReadAll(io.LimitReader(reader, maxFrameBytes+1))
	if err != nil || len(data) > maxFrameBytes {
		videoError(c, werrors.NewInternalServerError("Cannot read video keyframe"))
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(http.StatusOK, "image/jpeg", data)
}

func serveVideoRange(w http.ResponseWriter, req *http.Request, reader io.ReadCloser, name, mime string, size int64) error {
	raw := req.Header.Get("Range")
	if raw == "" || size <= 0 {
		return filetransport.Serve(w, req, reader, filetransport.Options{Filename: name, ContentType: mime, CacheControl: "private, no-store", Size: size})
	}
	defer reader.Close()
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", size))
	if !strings.HasPrefix(raw, "bytes=") || strings.Contains(raw, ",") {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}
	spec := strings.TrimPrefix(raw, "bytes=")
	pieces := strings.SplitN(spec, "-", 2)
	if len(pieces) != 2 {
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return nil
	}
	var start, end int64
	var err error
	if pieces[0] == "" {
		length, parseErr := strconv.ParseInt(pieces[1], 10, 64)
		if parseErr != nil || length <= 0 {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return nil
		}
		if length > size {
			length = size
		}
		start, end = size-length, size-1
	} else {
		start, err = strconv.ParseInt(pieces[0], 10, 64)
		if err != nil || start < 0 || start >= size {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return nil
		}
		end = size - 1
		if pieces[1] != "" {
			end, err = strconv.ParseInt(pieces[1], 10, 64)
			if err != nil || end < start {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return nil
			}
			if end >= size {
				end = size - 1
			}
		}
	}
	if seeker, ok := reader.(io.Seeker); ok {
		_, err = seeker.Seek(start, io.SeekStart)
	} else {
		_, err = io.CopyN(io.Discard, reader, start)
	}
	if err != nil {
		return err
	}
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, size))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(http.StatusPartialContent)
	_, err = io.CopyN(w, reader, end-start+1)
	return err
}
