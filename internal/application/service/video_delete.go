package service

import (
	"context"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

func (s *knowledgeService) cleanupVideoData(ctx context.Context, knowledge *types.Knowledge, files interfaces.FileService) error {
	if s.db == nil || knowledge == nil {
		return nil
	}
	filter := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ?", knowledge.TenantID, knowledge.ID)
	var frames []types.VideoKeyframe
	if err := filter.Find(&frames).Error; err != nil {
		return err
	}
	if files != nil {
		for _, frame := range frames {
			if err := files.DeleteFile(ctx, frame.FilePath); err != nil {
				logger.Warnf(ctx, "Failed to delete video keyframe %s: %v", frame.ID, err)
			}
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []any{&types.VideoKeyframe{}, &types.VideoEvidence{}, &types.VideoSegment{}, &types.VideoRun{}, &types.VideoAsset{}} {
			if err := tx.Where("tenant_id = ? AND knowledge_id = ?", knowledge.TenantID, knowledge.ID).Delete(table).Error; err != nil {
				return err
			}
		}
		return tx.Where("tenant_id = ? AND knowledge_id = ?", knowledge.TenantID, knowledge.ID).
			Delete(&types.VideoUpload{}).Error
	})
}
