CREATE TABLE video_assets (
    knowledge_id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    file_path TEXT NOT NULL,
    mime VARCHAR(100) NOT NULL,
    duration_ms BIGINT NOT NULL DEFAULT 0 CHECK (duration_ms >= 0),
    width INT NOT NULL DEFAULT 0 CHECK (width >= 0),
    height INT NOT NULL DEFAULT 0 CHECK (height >= 0),
    video_codec VARCHAR(100) NOT NULL DEFAULT '',
    audio_codec VARCHAR(100) NOT NULL DEFAULT '',
    has_audio BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(32) NOT NULL,
    active_run_id VARCHAR(36) NOT NULL DEFAULT '',
    current_run_id VARCHAR(36) NOT NULL DEFAULT '',
    failure_code VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_video_assets_tenant ON video_assets(tenant_id);

CREATE TABLE video_runs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    processor_version VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    completed_windows INT NOT NULL DEFAULT 0,
    total_windows INT NOT NULL DEFAULT 0,
    stage VARCHAR(32) NOT NULL DEFAULT 'queued',
    failure_code VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    published_at TIMESTAMPTZ
);
CREATE INDEX idx_video_runs_knowledge ON video_runs(tenant_id, knowledge_id, created_at);

CREATE TABLE video_segments (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    run_id VARCHAR(36) NOT NULL,
    start_ms BIGINT NOT NULL,
    end_ms BIGINT NOT NULL,
    chunk_id VARCHAR(36) NOT NULL DEFAULT '',
    CHECK (start_ms >= 0 AND end_ms > start_ms)
);
CREATE INDEX idx_video_segments_run_time ON video_segments(tenant_id, knowledge_id, run_id, start_ms, id);
CREATE INDEX idx_video_segments_chunk ON video_segments(chunk_id);

CREATE TABLE video_evidences (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    run_id VARCHAR(36) NOT NULL,
    segment_id VARCHAR(36) NOT NULL,
    modality VARCHAR(8) NOT NULL CHECK (modality IN ('asr', 'ocr', 'vlm')),
    start_ms BIGINT,
    end_ms BIGINT,
    frame_ms BIGINT,
    text TEXT NOT NULL,
    model_version VARCHAR(200) NOT NULL DEFAULT '',
    config_json TEXT NOT NULL DEFAULT '{}',
    CHECK (
        (modality = 'asr' AND start_ms >= 0 AND end_ms > start_ms AND frame_ms IS NULL)
        OR
        (modality IN ('ocr', 'vlm') AND frame_ms >= 0 AND start_ms IS NULL AND end_ms IS NULL)
    )
);
CREATE INDEX idx_video_evidences_segment ON video_evidences(tenant_id, run_id, segment_id);

CREATE TABLE video_keyframes (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL,
    run_id VARCHAR(36) NOT NULL,
    segment_id VARCHAR(36) NOT NULL,
    frame_ms BIGINT NOT NULL CHECK (frame_ms >= 0),
    file_path TEXT NOT NULL
);
CREATE INDEX idx_video_keyframes_segment ON video_keyframes(tenant_id, run_id, segment_id);
CREATE INDEX idx_video_keyframes_run_time ON video_keyframes(tenant_id, knowledge_id, run_id, frame_ms, id);
