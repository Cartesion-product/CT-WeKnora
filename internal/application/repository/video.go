package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type VideoRepository struct{ db *gorm.DB }

func NewVideoRepository(db *gorm.DB) *VideoRepository { return &VideoRepository{db: db} }

func (r *VideoRepository) GetAsset(ctx context.Context, tenantID uint64, knowledgeID string) (*types.VideoAsset, error) {
	var asset types.VideoAsset
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ?", tenantID, knowledgeID).First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func (r *VideoRepository) GetCurrentRun(ctx context.Context, tenantID uint64, knowledgeID string) (*types.VideoRun, error) {
	var asset types.VideoAsset
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ?", tenantID, knowledgeID).First(&asset).Error; err != nil {
		return nil, err
	}
	var run types.VideoRun
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND id = ?", tenantID, knowledgeID, asset.CurrentRunID).First(&run).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

func (r *VideoRepository) CreateUpload(ctx context.Context, upload *types.VideoUpload) error {
	return r.db.WithContext(ctx).Create(upload).Error
}

func (r *VideoRepository) GetUpload(ctx context.Context, tenantID uint64, kbID, id string) (*types.VideoUpload, []types.VideoUploadPart, error) {
	var upload types.VideoUpload
	if err := r.db.WithContext(ctx).Where("id = ? AND tenant_id = ? AND knowledge_base_id = ?", id, tenantID, kbID).First(&upload).Error; err != nil {
		return nil, nil, err
	}
	var parts []types.VideoUploadPart
	err := r.db.WithContext(ctx).Where("upload_id = ?", id).Order("part_number ASC").Find(&parts).Error
	return &upload, parts, err
}

func (r *VideoRepository) PutPart(ctx context.Context, uploadID string, part *types.VideoUploadPart) (string, error) {
	var previous types.VideoUploadPart
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var upload types.VideoUpload
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", uploadID, "uploading").First(&upload).Error; err != nil {
			return err
		}
		if err := tx.Where("upload_id = ? AND part_number = ?", uploadID, part.PartNumber).First(&previous).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		part.UploadID = uploadID
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "upload_id"}, {Name: "part_number"}},
			DoUpdates: clause.AssignmentColumns([]string{"size", "file_path"}),
		}).Create(part).Error
	})
	return previous.FilePath, err
}

func (r *VideoRepository) ClaimUpload(ctx context.Context, tenantID uint64, kbID, id string) (bool, error) {
	staleBefore := time.Now().Add(-30 * time.Minute)
	res := r.db.WithContext(ctx).Model(&types.VideoUpload{}).
		Where("id = ? AND tenant_id = ? AND knowledge_base_id = ?", id, tenantID, kbID).
		Where("status = ? OR (status = ? AND updated_at < ?)", "uploading", "completing", staleBefore).
		Updates(map[string]interface{}{"status": "completing", "updated_at": time.Now()})
	return res.RowsAffected == 1, res.Error
}

func (r *VideoRepository) GetActiveKeyframe(ctx context.Context, tenantID uint64, knowledgeID, frameID string) (*types.VideoKeyframe, error) {
	var frame types.VideoKeyframe
	err := r.db.WithContext(ctx).Table("video_keyframes AS frame").
		Select("frame.*").
		Joins("JOIN video_assets AS asset ON asset.knowledge_id = frame.knowledge_id AND asset.tenant_id = frame.tenant_id AND asset.active_run_id = frame.run_id").
		Where("frame.id = ? AND frame.tenant_id = ? AND frame.knowledge_id = ?", frameID, tenantID, knowledgeID).
		Take(&frame).Error
	if err != nil {
		return nil, err
	}
	return &frame, nil
}

func (r *VideoRepository) ResetUpload(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Model(&types.VideoUpload{}).Where("id = ? AND status = ?", id, "completing").
		Updates(map[string]interface{}{"status": "uploading", "updated_at": time.Now()}).Error
}

func (r *VideoRepository) CompleteUpload(ctx context.Context, upload *types.VideoUpload, knowledge *types.Knowledge, asset *types.VideoAsset, run *types.VideoRun, tagIDs []string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if len(tagIDs) > 0 {
			if err := tx.Model(&types.KnowledgeTag{}).Where("tenant_id = ? AND knowledge_base_id = ? AND id IN ?", upload.TenantID, upload.KnowledgeBaseID, tagIDs).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(tagIDs)) {
				return errors.New("invalid video tag IDs")
			}
		}
		if err := tx.Create(knowledge).Error; err != nil {
			return err
		}
		if err := tx.Create(asset).Error; err != nil {
			return err
		}
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		quota := tx.Model(&types.Tenant{}).Where("id = ? AND (storage_quota <= 0 OR storage_used <= storage_quota - ?)", upload.TenantID, upload.Size).
			Update("storage_used", gorm.Expr("storage_used + ?", upload.Size))
		if quota.Error != nil {
			return quota.Error
		}
		if quota.RowsAffected != 1 {
			return errors.New("workspace storage quota exceeded")
		}
		for _, id := range tagIDs {
			if err := tx.Create(&types.KnowledgeTagRelation{KnowledgeID: knowledge.ID, TagID: id, CreatedAt: time.Now()}).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&types.VideoUpload{}).Where("id = ? AND status = ?", upload.ID, "completing").
			Updates(map[string]interface{}{"status": "completed", "knowledge_id": knowledge.ID, "updated_at": time.Now()})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("video upload state changed")
		}
		return nil
	})
}

func (r *VideoRepository) DeleteUpload(ctx context.Context, tenantID uint64, kbID, id string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var upload types.VideoUpload
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND tenant_id = ? AND knowledge_base_id = ? AND status = ?", id, tenantID, kbID, "uploading").First(&upload).Error; err != nil {
			return err
		}
		if err := tx.Where("upload_id = ?", id).Delete(&types.VideoUploadPart{}).Error; err != nil {
			return err
		}
		res := tx.Where("id = ? AND tenant_id = ? AND knowledge_base_id = ? AND status = ?", id, tenantID, kbID, "uploading").Delete(&types.VideoUpload{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errors.New("video upload state changed")
		}
		return nil
	})
}

func (r *VideoRepository) GetVideoStatus(ctx context.Context, tenantID uint64, knowledgeID string) (*types.VideoAsset, *types.VideoRun, error) {
	asset, err := r.GetAsset(ctx, tenantID, knowledgeID)
	if err != nil {
		return nil, nil, err
	}
	var run *types.VideoRun
	if asset.CurrentRunID != "" {
		var row types.VideoRun
		if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND id = ?",
			tenantID, knowledgeID, asset.CurrentRunID).First(&row).Error; err != nil {
			return nil, nil, err
		}
		run = &row
	}
	return asset, run, nil
}

func (r *VideoRepository) GetVideoPage(ctx context.Context, tenantID uint64, knowledgeID string,
	offset, limit int, segmentID string, nextOffset *int) (*types.VideoAsset, *types.VideoRun, []types.VideoSegment, []types.VideoEvidence, []types.VideoKeyframe, error) {
	asset, _, err := r.GetVideoStatus(ctx, tenantID, knowledgeID)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	runID := asset.ActiveRunID
	if runID == "" {
		runID = asset.CurrentRunID
	}
	var run *types.VideoRun
	segments := []types.VideoSegment{}
	evidences := []types.VideoEvidence{}
	keyframes := []types.VideoKeyframe{}
	if runID != "" {
		var row types.VideoRun
		if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND id = ?", tenantID, knowledgeID, runID).First(&row).Error; err != nil {
			return nil, nil, nil, nil, nil, err
		}
		run = &row
		if segmentID != "" {
			var target types.VideoSegment
			if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ? AND id = ?",
				tenantID, knowledgeID, runID, segmentID).First(&target).Error; err != nil {
				return nil, nil, nil, nil, nil, err
			}
			var preceding int64
			if err := r.db.WithContext(ctx).Model(&types.VideoSegment{}).
				Where("tenant_id = ? AND knowledge_id = ? AND run_id = ? AND (start_ms < ? OR (start_ms = ? AND id < ?))",
					tenantID, knowledgeID, runID, target.StartMS, target.StartMS, target.ID).Count(&preceding).Error; err != nil {
				return nil, nil, nil, nil, nil, err
			}
			offset = int(preceding)
		}
		if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ?", tenantID, knowledgeID, runID).
			Order("start_ms, id").Offset(offset).Limit(limit + 1).Find(&segments).Error; err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if len(segments) > limit {
			segments = segments[:limit]
			if nextOffset != nil {
				*nextOffset = offset + limit
			}
		}
		if len(segments) == 0 {
			return asset, run, segments, evidences, keyframes, nil
		}
		segmentIDs := make([]string, 0, len(segments))
		for _, segment := range segments {
			segmentIDs = append(segmentIDs, segment.ID)
		}
		if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ? AND segment_id IN ?",
			tenantID, knowledgeID, runID, segmentIDs).Order("segment_id, id").Find(&evidences).Error; err != nil {
			return nil, nil, nil, nil, nil, err
		}
		if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ? AND segment_id IN ?",
			tenantID, knowledgeID, runID, segmentIDs).Order("frame_ms, id").Find(&keyframes).Error; err != nil {
			return nil, nil, nil, nil, nil, err
		}
	}
	return asset, run, segments, evidences, keyframes, nil
}
