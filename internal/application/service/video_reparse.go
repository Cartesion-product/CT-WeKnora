package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	werrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *knowledgeService) reparseVideoKnowledge(ctx context.Context, knowledge *types.Knowledge,
	kb *types.KnowledgeBase, overrides *types.KnowledgeProcessOverrides) (*types.Knowledge, error) {
	if os.Getenv("VIDEO_INGEST_ENABLED") != "1" {
		return nil, werrors.NewForbiddenError("Video ingestion is disabled")
	}
	if s.db == nil || s.task == nil {
		return nil, werrors.NewInternalServerError("Video processing is unavailable")
	}
	if overrides != nil {
		if err := ValidateProcessOverrides(ctx, kb, overrides, []string{"mp4"}); err != nil {
			return nil, err
		}
		if err := knowledge.SetProcessOverrides(overrides); err != nil {
			return nil, err
		}
	}
	runID := uuid.NewString()
	previousRunID := ""
	previousStatus := ""
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var asset types.VideoAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND knowledge_id = ?", knowledge.TenantID, knowledge.ID).
			First(&asset).Error; err != nil {
			return err
		}
		if asset.CurrentRunID != "" {
			var current types.VideoRun
			if err := tx.Where("id = ? AND tenant_id = ?", asset.CurrentRunID, knowledge.TenantID).
				First(&current).Error; err != nil {
				return err
			}
			if current.Status == "queued" || current.Status == "processing" {
				return werrors.NewBadRequestError("Video processing is still running")
			}
		}
		previousRunID, previousStatus = asset.CurrentRunID, asset.Status
		run := types.VideoRun{ID: runID, TenantID: knowledge.TenantID,
			KnowledgeID: knowledge.ID, ProcessorVersion: videoProcessorVersion,
			Status: "queued", Stage: "queued", CreatedAt: time.Now()}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		if err := tx.Model(&asset).Updates(map[string]any{
			"current_run_id": runID, "status": "queued", "failure_code": "",
		}).Error; err != nil {
			return err
		}
		if overrides != nil {
			if err := tx.Model(&types.Knowledge{}).Where("id = ? AND tenant_id = ?", knowledge.ID, knowledge.TenantID).
				Update("metadata", knowledge.Metadata).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(VideoProcessPayload{
		TenantID: knowledge.TenantID, KnowledgeID: knowledge.ID, ProcessingRunID: runID,
	})
	if err == nil {
		_, err = s.task.Enqueue(asynq.NewTask(types.TypeVideoProcess, payload,
			asynq.TaskID(runID), asynq.Queue(types.QueueMedia), asynq.MaxRetry(3), asynq.Timeout(4*time.Hour)))
	}
	if err != nil && !errors.Is(err, asynq.ErrTaskIDConflict) {
		rollbackErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&types.VideoAsset{}).Where("knowledge_id = ? AND current_run_id = ?", knowledge.ID, runID).
				Updates(map[string]any{"current_run_id": previousRunID, "status": previousStatus}).Error; err != nil {
				return err
			}
			return tx.Where("id = ?", runID).Delete(&types.VideoRun{}).Error
		})
		if rollbackErr != nil {
			return nil, fmt.Errorf("queue video processing: %w (rollback: %v)", err, rollbackErr)
		}
		return nil, werrors.NewInternalServerError("Failed to queue video processing")
	}
	return knowledge, nil
}

// A rebuild can be cancelled while the published version stays available.
func (s *knowledgeService) cancelVideoRebuild(ctx context.Context, knowledge *types.Knowledge) (bool, error) {
	if s.db == nil || !IsVideoType(knowledge.FileType) {
		return false, nil
	}
	var cancelled bool
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var asset types.VideoAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("tenant_id = ? AND knowledge_id = ?", knowledge.TenantID, knowledge.ID).First(&asset).Error; err != nil {
			return err
		}
		if asset.ActiveRunID == "" || asset.CurrentRunID == asset.ActiveRunID {
			if asset.ActiveRunID != "" && asset.CurrentRunID == asset.ActiveRunID {
				var run types.VideoRun
				if err := tx.Where("id = ? AND tenant_id = ?", asset.CurrentRunID, knowledge.TenantID).
					First(&run).Error; err != nil {
					return err
				}
				if run.Status == "processing" {
					return werrors.NewBadRequestError("Video version is being published; retry cancellation after publishing finishes")
				}
			}
			return nil
		}
		var run types.VideoRun
		if err := tx.Where("id = ? AND tenant_id = ?", asset.CurrentRunID, knowledge.TenantID).
			First(&run).Error; err != nil {
			return err
		}
		if run.Status != "queued" && run.Status != "processing" {
			return nil
		}
		if err := tx.Model(&run).Updates(map[string]any{"status": "cancelled", "stage": "cancelled"}).Error; err != nil {
			return err
		}
		if err := tx.Model(&asset).Updates(map[string]any{
			"current_run_id": asset.ActiveRunID, "status": "completed", "failure_code": "",
		}).Error; err != nil {
			return err
		}
		cancelled = true
		return nil
	})
	if err != nil || !cancelled {
		return cancelled, err
	}
	s.dequeueKnowledgeTasks(ctx, knowledge.ID)
	return true, nil
}
