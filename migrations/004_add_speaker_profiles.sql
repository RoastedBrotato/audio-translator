-- Persist speaker diarization profiles across restarts
CREATE TABLE IF NOT EXISTS speaker_profiles (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(100) NOT NULL,
    profile_id VARCHAR(50) NOT NULL,
    embedding JSONB NOT NULL,
    count INTEGER NOT NULL DEFAULT 1,
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(session_id, profile_id)
);

CREATE INDEX IF NOT EXISTS idx_speaker_profiles_session ON speaker_profiles(session_id);
