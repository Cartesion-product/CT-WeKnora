CREATE TABLE video_assets (
    knowledge_id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    mime TEXT NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
    height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    has_audio INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    active_run_id TEXT NOT NULL DEFAULT '',
    current_run_id TEXT NOT NULL DEFAULT '',
    failure_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX idx_video_assets_tenant ON video_assets(tenant_id);

CREATE TABLE video_runs (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_id TEXT NOT NULL,
    processor_version TEXT NOT NULL,
    status TEXT NOT NULL,
    completed_windows INTEGER NOT NULL DEFAULT 0,
    total_windows INTEGER NOT NULL DEFAULT 0,
    stage TEXT NOT NULL DEFAULT 'queued',
    failure_code TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    published_at DATETIME
);
CREATE INDEX idx_video_runs_knowledge ON video_runs(tenant_id, knowledge_id, created_at);

CREATE TABLE video_segments (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    start_ms INTEGER NOT NULL,
    end_ms INTEGER NOT NULL,
    chunk_id TEXT NOT NULL DEFAULT '',
    CHECK (start_ms >= 0 AND end_ms > start_ms)
);
CREATE INDEX idx_video_segments_run_time ON video_segments(tenant_id, knowledge_id, run_id, start_ms, id);
CREATE INDEX idx_video_segments_chunk ON video_segments(chunk_id);

CREATE TABLE video_evidences (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    segment_id TEXT NOT NULL,
    modality TEXT NOT NULL CHECK (modality IN ('asr', 'ocr', 'vlm')),
    start_ms INTEGER,
    end_ms INTEGER,
    frame_ms INTEGER,
    text TEXT NOT NULL,
    model_version TEXT NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    CHECK (
        (modality = 'asr' AND start_ms >= 0 AND end_ms > start_ms AND frame_ms IS NULL)
        OR
        (modality IN ('ocr', 'vlm') AND frame_ms >= 0 AND start_ms IS NULL AND end_ms IS NULL)
    )
);
CREATE INDEX idx_video_evidences_segment ON video_evidences(tenant_id, run_id, segment_id);

CREATE TABLE video_keyframes (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    segment_id TEXT NOT NULL,
    frame_ms INTEGER NOT NULL CHECK (frame_ms >= 0),
    file_path TEXT NOT NULL
);
CREATE INDEX idx_video_keyframes_segment ON video_keyframes(tenant_id, run_id, segment_id);
CREATE INDEX idx_video_keyframes_run_time ON video_keyframes(tenant_id, knowledge_id, run_id, frame_ms, id);
