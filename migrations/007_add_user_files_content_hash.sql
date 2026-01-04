-- Store content hash for per-user file de-duplication
ALTER TABLE user_files ADD COLUMN IF NOT EXISTS content_hash VARCHAR(128);
CREATE INDEX IF NOT EXISTS idx_user_files_content_hash ON user_files(user_id, content_hash);
