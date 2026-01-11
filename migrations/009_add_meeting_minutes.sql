-- Migration 009: Add meeting minutes storage

CREATE TABLE IF NOT EXISTS meeting_minutes (
    id SERIAL PRIMARY KEY,
    meeting_id VARCHAR(50) NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    language VARCHAR(10) NOT NULL DEFAULT 'en',
    content JSONB NOT NULL,
    summary TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),

    UNIQUE(meeting_id, language)
);

CREATE INDEX IF NOT EXISTS idx_meeting_minutes_meeting ON meeting_minutes(meeting_id, language);

COMMENT ON TABLE meeting_minutes IS 'Generated meeting minutes (participants, key points, action items, decisions, summary)';
