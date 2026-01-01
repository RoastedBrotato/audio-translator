-- Store host token for meeting end control
ALTER TABLE meetings ADD COLUMN IF NOT EXISTS host_token VARCHAR(128);
CREATE INDEX IF NOT EXISTS idx_meetings_host_token ON meetings(host_token);
