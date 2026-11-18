package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// cloneVideoData maps the active media run to the IDs created by CloneChunk.
// The caller owns cleanup of returned copied objects if any later step fails.
func (s *knowledgeService) cloneVideoData(ctx context.Context, src, dst *types.Knowledge,
	sourceKB, targetKB *types.KnowledgeBase, chunkIDs map[string]string) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("video metadata database unavailable")
	}
	var source types.VideoAsset
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ?", src.TenantID, src.ID).
		First(&source).Error; err != nil {
		return nil, err
	}
	if source.ActiveRunID == "" || source.Status == "queued" || source.Status == "processing" {
		return nil, fmt.Errorf("source video has no stable published run")
	}
	var segments []types.VideoSegment
	var evidences []types.VideoEvidence
	var frames []types.VideoKeyframe
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ?", src.TenantID, src.ID, source.ActiveRunID).
		Find(&segments).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ?", src.TenantID, src.ID, source.ActiveRunID).
		Find(&evidences).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_id = ? AND run_id = ?", src.TenantID, src.ID, source.ActiveRunID).
		Find(&frames).Error; err != nil {
		return nil, err
	}
	runID := uuid.NewString()
	segmentIDs := make(map[string]string, len(segments))
	for i := range segments {
		oldID := segments[i].ID
		segmentIDs[oldID] = uuid.NewString()
		if segments[i].ChunkID != "" {
			mapped := chunkIDs[segments[i].ChunkID]
			if mapped == "" {
				return nil, fmt.Errorf("video segment %s has no cloned chunk", oldID)
			}
			segments[i].ChunkID = mapped
		}
		segments[i].ID = segmentIDs[oldID]
		segments[i].TenantID, segments[i].KnowledgeID, segments[i].RunID = dst.TenantID, dst.ID, runID
	}
	for i := range evidences {
		mapped := segmentIDs[evidences[i].SegmentID]
		if mapped == "" {
			return nil, fmt.Errorf("video evidence has no cloned segment")
		}
		evidences[i].ID = uuid.NewString()
		evidences[i].TenantID, evidences[i].KnowledgeID, evidences[i].RunID = dst.TenantID, dst.ID, runID
		evidences[i].SegmentID = mapped
	}
	dstSvc := s.resolveFileService(ctx, targetKB)
	copied := make([]string, 0, len(frames))
	for i := range frames {
		mapped := segmentIDs[frames[i].SegmentID]
		if mapped == "" {
			return copied, fmt.Errorf("video frame has no cloned segment")
		}
		srcSvc := s.resolveFileServiceForPath(ctx, sourceKB, frames[i].FilePath)
		path, err := copyOwnedObject(ctx, srcSvc, dstSvc, frames[i].FilePath, dst.TenantID,
			dst.ID, s.resourceCatalog, types.ResourceRelationArtifact)
		if err != nil {
			return copied, err
		}
		copied = append(copied, path)
		frames[i].ID = uuid.NewString()
		frames[i].TenantID, frames[i].KnowledgeID, frames[i].RunID = dst.TenantID, dst.ID, runID
		frames[i].SegmentID, frames[i].FilePath = mapped, path
	}
	now := time.Now()
	source.KnowledgeID, source.TenantID, source.FilePath = dst.ID, dst.TenantID, dst.FilePath
	source.ActiveRunID, source.CurrentRunID = runID, runID
	source.Status, source.FailureCode = "completed", ""
	source.CreatedAt, source.UpdatedAt = now, now
	run := types.VideoRun{ID: runID, TenantID: dst.TenantID, KnowledgeID: dst.ID,
		ProcessorVersion: videoProcessorVersion, Status: "completed", Stage: "completed",
		CreatedAt: now, PublishedAt: &now}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&source).Error; err != nil {
			return err
		}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if len(segments) > 0 {
			if err := tx.Create(&segments).Error; err != nil {
				return err
			}
		}
		if len(evidences) > 0 {
			if err := tx.Create(&evidences).Error; err != nil {
				return err
			}
		}
		if len(frames) > 0 {
			return tx.Create(&frames).Error
		}
		return nil
	})
	return copied, err
}
