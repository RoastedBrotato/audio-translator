-- Store final transcript snapshots per meeting/language
CREATE TABLE IF NOT EXISTS meeting_transcript_snapshots (
    id SERIAL PRIMARY KEY,
    meeting_id VARCHAR(50) REFERENCES meetings(id) ON DELETE CASCADE,
    language VARCHAR(10) NOT NULL,
    transcript TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(meeting_id, language)
);

CREATE INDEX IF NOT EXISTS idx_transcript_snapshots_meeting ON meeting_transcript_snapshots(meeting_id);
