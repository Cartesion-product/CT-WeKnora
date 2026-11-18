ALTER TABLE knowledge_bases ADD COLUMN video_config JSON NOT NULL DEFAULT '{"frame_interval_seconds":15,"max_frames":120,"ocr_language":"chi_sim+eng"}';
