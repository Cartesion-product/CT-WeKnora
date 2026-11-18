CREATE TABLE video_uploads (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    size BIGINT NOT NULL,
    mime VARCHAR(100) NOT NULL,
    folder_path VARCHAR(1024) NOT NULL DEFAULT '',
    tag_ids_json TEXT NOT NULL DEFAULT '[]',
    process_config_json TEXT NOT NULL DEFAULT '',
    status VARCHAR(32) NOT NULL,
    knowledge_id VARCHAR(36) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_video_uploads_scope ON video_uploads(tenant_id, knowledge_base_id);
CREATE TABLE video_upload_parts (
    upload_id VARCHAR(36) NOT NULL,
    part_number INT NOT NULL,
    size BIGINT NOT NULL,
    file_path TEXT NOT NULL,
    PRIMARY KEY(upload_id, part_number)
);
