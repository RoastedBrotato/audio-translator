-- Performance optimization indexes
-- This migration adds indexes that significantly improve query performance

-- Index for meeting creator queries (used in history filtering)
CREATE INDEX IF NOT EXISTS idx_meetings_created_by ON meetings(created_by);

-- Index for participant user lookups (used in joins and access checks)
CREATE INDEX IF NOT EXISTS idx_meeting_participants_user_id ON meeting_participants(user_id);

-- Composite index for meeting + user participant queries (most common query pattern)
CREATE INDEX IF NOT EXISTS idx_meeting_participants_meeting_user ON meeting_participants(meeting_id, user_id);

-- Index for transcript snapshot language lookups (used in bulk language fetching)
CREATE INDEX IF NOT EXISTS idx_transcript_snapshots_meeting_id ON meeting_transcript_snapshots(meeting_id);

-- Index for meeting status + created_at for efficient sorting
CREATE INDEX IF NOT EXISTS idx_meetings_active_created_at ON meetings(is_active, created_at DESC);

-- Index for RAG chunk queries
CREATE INDEX IF NOT EXISTS idx_meeting_chunks_meeting_status ON meeting_chunks(meeting_id, processing_status);

-- Index for meeting minutes lookups
CREATE INDEX IF NOT EXISTS idx_meeting_minutes_meeting_lang ON meeting_minutes(meeting_id, language);
