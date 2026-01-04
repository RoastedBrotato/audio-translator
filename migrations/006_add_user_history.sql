-- Add user history tracking for uploads, recordings, and streaming sessions

ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(255);
ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS last_login TIMESTAMP;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique ON users(email);

CREATE TABLE IF NOT EXISTS user_video_sessions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_id VARCHAR(100) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    transcription TEXT,
    translation TEXT,
    video_path TEXT,
    audio_path TEXT,
    tts_path TEXT,
    source_lang VARCHAR(10),
    target_lang VARCHAR(10),
    duration_seconds INTEGER,
    expires_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_audio_sessions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_id VARCHAR(100) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    transcription TEXT,
    translation TEXT,
    audio_path TEXT,
    source_lang VARCHAR(10),
    target_lang VARCHAR(10),
    has_diarization BOOLEAN DEFAULT false,
    num_speakers INTEGER,
    segments JSONB,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_streaming_sessions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_id VARCHAR(100) NOT NULL,
    source_lang VARCHAR(10),
    target_lang VARCHAR(10),
    total_chunks INTEGER DEFAULT 0,
    total_duration_seconds INTEGER DEFAULT 0,
    final_transcript TEXT,
    final_translation TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_files (
    id SERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    session_type VARCHAR(30) NOT NULL,
    session_id VARCHAR(100) NOT NULL,
    bucket_name VARCHAR(255) NOT NULL,
    file_key TEXT NOT NULL,
    etag VARCHAR(255),
    mime_type VARCHAR(100),
    file_size_bytes BIGINT,
    accessed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE (bucket_name, file_key)
);

CREATE TABLE IF NOT EXISTS keycloak_users (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    keycloak_sub VARCHAR(255) NOT NULL UNIQUE,
    preferred_username VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_video_sessions_user_created ON user_video_sessions(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_video_sessions_session_id ON user_video_sessions(session_id);

CREATE INDEX IF NOT EXISTS idx_user_audio_sessions_user_created ON user_audio_sessions(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_audio_sessions_session_id ON user_audio_sessions(session_id);
CREATE INDEX IF NOT EXISTS idx_user_audio_sessions_segments ON user_audio_sessions USING GIN (segments);

CREATE INDEX IF NOT EXISTS idx_user_streaming_sessions_user_created ON user_streaming_sessions(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_user_streaming_sessions_session_id ON user_streaming_sessions(session_id);

CREATE INDEX IF NOT EXISTS idx_user_files_session ON user_files(session_type, session_id);
CREATE INDEX IF NOT EXISTS idx_user_files_user_created ON user_files(user_id, created_at);

CREATE INDEX IF NOT EXISTS idx_keycloak_users_user_id ON keycloak_users(user_id);
