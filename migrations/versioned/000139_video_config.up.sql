ALTER TABLE knowledge_bases ADD COLUMN IF NOT EXISTS video_config JSONB NOT NULL DEFAULT '{"frame_interval_seconds":15,"max_frames":120,"ocr_language":"chi_sim+eng"}';
