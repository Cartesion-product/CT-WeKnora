package types

import (
	"fmt"
	"strings"
	"time"
)

const (
	VideoModalityASR = "asr"
	VideoModalityOCR = "ocr"
	VideoModalityVLM = "vlm"
)

// VideoAsset is the current media state for one existing Knowledge entry.
// ActiveRunID is the only published run; a processing run never becomes
// searchable merely because some of its segments have been indexed.
type VideoAsset struct {
	KnowledgeID  string    `json:"knowledge_id" gorm:"type:varchar(36);primaryKey"`
	TenantID     uint64    `json:"tenant_id"`
	FilePath     string    `json:"-" gorm:"type:text"`
	MIME         string    `json:"mime" gorm:"type:varchar(100)"`
	DurationMS   int64     `json:"duration_ms"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	VideoCodec   string    `json:"video_codec" gorm:"type:varchar(100)"`
	AudioCodec   string    `json:"audio_codec,omitempty" gorm:"type:varchar(100)"`
	HasAudio     bool      `json:"has_audio"`
	Status       string    `json:"status" gorm:"type:varchar(32)"`
	ActiveRunID  string    `json:"active_run_id,omitempty" gorm:"type:varchar(36)"`
	CurrentRunID string    `json:"current_run_id,omitempty" gorm:"type:varchar(36)"`
	FailureCode  string    `json:"failure_code,omitempty" gorm:"type:varchar(100)"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// VideoListInfo keeps the knowledge list compact while exposing media state.
type VideoListInfo struct {
	DurationMS  int64  `json:"duration_ms"`
	Status      string `json:"status"`
	Stage       string `json:"stage,omitempty"`
	ThumbnailID string `json:"thumbnail_id,omitempty"`
}

type VideoRun struct {
	ID               string     `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID         uint64     `json:"tenant_id"`
	KnowledgeID      string     `json:"knowledge_id" gorm:"type:varchar(36)"`
	ProcessorVersion string     `json:"processor_version" gorm:"type:varchar(64)"`
	Status           string     `json:"status" gorm:"type:varchar(32)"`
	CompletedWindows int        `json:"completed_windows"`
	TotalWindows     int        `json:"total_windows"`
	Stage            string     `json:"stage" gorm:"type:varchar(32)"`
	FailureCode      string     `json:"failure_code,omitempty" gorm:"type:varchar(100)"`
	CreatedAt        time.Time  `json:"created_at"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
}

type VideoSegment struct {
	ID          string `json:"segment_id" gorm:"type:varchar(36);primaryKey"`
	TenantID    uint64 `json:"tenant_id"`
	KnowledgeID string `json:"knowledge_id" gorm:"type:varchar(36)"`
	RunID       string `json:"processing_run_id" gorm:"type:varchar(36)"`
	StartMS     int64  `json:"start_ms"`
	EndMS       int64  `json:"end_ms"`
	ChunkID     string `json:"chunk_id,omitempty" gorm:"type:varchar(36)"`
}

func (s VideoSegment) Validate(durationMS int64) error {
	if durationMS <= 0 || s.StartMS < 0 || s.EndMS <= s.StartMS || s.EndMS > durationMS {
		return fmt.Errorf("video segment is outside the source duration")
	}
	return nil
}

// VideoEvidence describes exactly where a statement came from. ASR uses a
// half-open [StartMS, EndMS) window; OCR/VLM use FrameMS only.
type VideoEvidence struct {
	ID           string `json:"evidence_id" gorm:"type:varchar(36);primaryKey"`
	TenantID     uint64 `json:"tenant_id"`
	KnowledgeID  string `json:"knowledge_id" gorm:"type:varchar(36)"`
	RunID        string `json:"processing_run_id" gorm:"type:varchar(36)"`
	SegmentID    string `json:"segment_id" gorm:"type:varchar(36)"`
	Modality     string `json:"modality" gorm:"type:varchar(8)"`
	StartMS      *int64 `json:"start_ms,omitempty"`
	EndMS        *int64 `json:"end_ms,omitempty"`
	FrameMS      *int64 `json:"frame_ms,omitempty"`
	Text         string `json:"text" gorm:"type:text"`
	ModelVersion string `json:"model_version,omitempty" gorm:"type:varchar(200)"`
	ConfigJSON   string `json:"-" gorm:"column:config_json;type:text"`
}

func (e VideoEvidence) Validate(durationMS int64) error {
	if durationMS <= 0 || strings.TrimSpace(e.Text) == "" {
		return fmt.Errorf("video evidence has no source duration or text")
	}
	switch e.Modality {
	case VideoModalityASR:
		if e.StartMS == nil || e.EndMS == nil || e.FrameMS != nil ||
			*e.StartMS < 0 || *e.EndMS <= *e.StartMS || *e.EndMS > durationMS {
			return fmt.Errorf("ASR evidence must have a valid source window")
		}
	case VideoModalityOCR, VideoModalityVLM:
		if e.FrameMS == nil || e.StartMS != nil || e.EndMS != nil ||
			*e.FrameMS < 0 || *e.FrameMS >= durationMS {
			return fmt.Errorf("frame evidence must have one valid frame position")
		}
	default:
		return fmt.Errorf("unknown video evidence modality %q", e.Modality)
	}
	return nil
}

type VideoKeyframe struct {
	ID          string `json:"id" gorm:"type:varchar(36);primaryKey"`
	TenantID    uint64 `json:"tenant_id"`
	KnowledgeID string `json:"knowledge_id" gorm:"type:varchar(36)"`
	RunID       string `json:"processing_run_id" gorm:"type:varchar(36)"`
	SegmentID   string `json:"segment_id" gorm:"type:varchar(36)"`
	FrameMS     int64  `json:"frame_ms"`
	FilePath    string `json:"-" gorm:"type:text"`
}

// MediaReference is optional on search and persisted chat references. A
// client must resolve playable URLs through a fresh authorized request.
type MediaReference struct {
	Type            string `json:"type"`
	SegmentID       string `json:"segment_id"`
	EvidenceID      string `json:"evidence_id"`
	Modality        string `json:"modality"`
	ProcessingRunID string `json:"processing_run_id"`
	StartMS         *int64 `json:"start_ms,omitempty"`
	EndMS           *int64 `json:"end_ms,omitempty"`
	FrameMS         *int64 `json:"frame_ms,omitempty"`
}
