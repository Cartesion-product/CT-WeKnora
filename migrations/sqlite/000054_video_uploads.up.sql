CREATE TABLE video_uploads (
    id TEXT PRIMARY KEY,
    tenant_id INTEGER NOT NULL,
    knowledge_base_id TEXT NOT NULL,
    file_name TEXT NOT NULL,
    size INTEGER NOT NULL,
    mime TEXT NOT NULL,
    folder_path TEXT NOT NULL DEFAULT '',
    tag_ids_json TEXT NOT NULL DEFAULT '[]',
    process_config_json TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    knowledge_id TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX idx_video_uploads_scope ON video_uploads(tenant_id, knowledge_base_id);
CREATE TABLE video_upload_parts (
    upload_id TEXT NOT NULL,
    part_number INTEGER NOT NULL,
    size INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    PRIMARY KEY(upload_id, part_number)
);
