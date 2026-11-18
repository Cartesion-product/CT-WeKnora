package repository

import (
	"context"
	"fmt"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"gorm.io/gorm"
)

type graphSearchEvidenceRepository struct {
	db *gorm.DB
}

// NewGraphSearchEvidenceRepository constructs the scoped live-source projection.
func NewGraphSearchEvidenceRepository(db *gorm.DB) interfaces.GraphSearchEvidenceRepository {
	return &graphSearchEvidenceRepository{db: db}
}

func (r *graphSearchEvidenceRepository) GetGraphSourceChunks(
	ctx context.Context, tenantID uint64, kbID string, ids []string,
) ([]types.GraphSearchSourceChunk, error) {
	if tenantID == 0 || kbID == "" || len(ids) > types.GraphSearchMaxEvidence {
		return nil, fmt.Errorf("invalid graph evidence scope or budget")
	}
	rows := []types.GraphSearchSourceChunk{}
	if len(ids) == 0 {
		return rows, nil
	}
	// Explicit soft-delete predicates are necessary: Table/Joins do not apply
	// the two models' GORM default scopes. SUBSTR is supported by every SQL
	// backend; fetching one extra character detects clipping without loading
	// unbounded content or doing a second, racy metadata-to-content lookup.
	err := r.db.WithContext(ctx).Table("chunks AS c").
		Select(`c.id, c.tenant_id, c.knowledge_base_id, c.knowledge_id,
			c.chunk_index, c.start_at, c.end_at,
			SUBSTR(c.content, 1, ?) AS content, SUBSTR(k.title, 1, 513) AS title`,
			types.GraphSearchMaxContentRunes+1).
		Joins(`JOIN knowledges AS k ON k.id = c.knowledge_id
			AND k.tenant_id = c.tenant_id AND k.knowledge_base_id = c.knowledge_base_id`).
		Where("c.tenant_id = ? AND c.knowledge_base_id = ? AND c.id IN ?", tenantID, kbID, ids).
		Where("c.deleted_at IS NULL AND k.deleted_at IS NULL AND c.is_enabled = ?", true).
		Where("k.enable_status = ? AND k.parse_status <> ?", "enabled", types.ParseStatusDeleting).
		Where("(c.index_status = ? OR c.index_status = ?)", "ready", "").
		Order("c.id").Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].Content, rows[i].ContentTruncated = graphSearchClip(rows[i].Content, types.GraphSearchMaxContentRunes)
		rows[i].Title, rows[i].TitleTruncated = graphSearchClip(rows[i].Title, 512)
	}
	if err := r.attachGraphVideoMedia(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *graphSearchEvidenceRepository) attachGraphVideoMedia(ctx context.Context, rows []types.GraphSearchSourceChunk) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	type mediaRow struct {
		ChunkID    string
		SegmentID  string
		RunID      string
		EvidenceID string
		Modality   string
		StartMS    *int64
		EndMS      *int64
		FrameMS    *int64
	}
	var mediaRows []mediaRow
	if err := r.db.WithContext(ctx).Table("video_segments AS segment").
		Select("segment.chunk_id, segment.id AS segment_id, segment.run_id, evidence.id AS evidence_id, evidence.modality, evidence.start_ms, evidence.end_ms, evidence.frame_ms").
		Joins("JOIN video_assets AS asset ON asset.knowledge_id = segment.knowledge_id AND asset.tenant_id = segment.tenant_id AND asset.active_run_id = segment.run_id").
		Joins("JOIN video_evidences AS evidence ON evidence.segment_id = segment.id AND evidence.run_id = segment.run_id").
		Where("segment.chunk_id IN ?", ids).Order("segment.chunk_id, evidence.modality, evidence.id").Scan(&mediaRows).Error; err != nil {
		return err
	}
	byChunk := make(map[string]*types.MediaReference, len(mediaRows))
	for _, row := range mediaRows {
		if _, exists := byChunk[row.ChunkID]; exists {
			continue
		}
		byChunk[row.ChunkID] = &types.MediaReference{Type: "video", SegmentID: row.SegmentID,
			EvidenceID: row.EvidenceID, Modality: row.Modality, ProcessingRunID: row.RunID,
			StartMS: row.StartMS, EndMS: row.EndMS, FrameMS: row.FrameMS}
	}
	for i := range rows {
		rows[i].Media = byChunk[rows[i].ID]
	}
	return nil
}

func graphSearchClip(text string, limit int) (string, bool) {
	runes := []rune(text) // SQL has already bounded this allocation.
	if len(runes) > limit {
		return string(runes[:limit]), true
	}
	return text, false
}
