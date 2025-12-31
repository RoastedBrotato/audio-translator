-- Add meeting mode column
ALTER TABLE meetings ADD COLUMN IF NOT EXISTS mode VARCHAR(20) DEFAULT 'individual';

-- Add speaker mappings table for shared room mode
CREATE TABLE IF NOT EXISTS speaker_mappings (
    id SERIAL PRIMARY KEY,
    meeting_id VARCHAR(50) REFERENCES meetings(id) ON DELETE CASCADE,
    speaker_id VARCHAR(20) NOT NULL,
    speaker_name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(meeting_id, speaker_id)
);

CREATE INDEX IF NOT EXISTS idx_speaker_mappings_meeting ON speaker_mappings(meeting_id);
