-- Meeting Access Control (Role-Based ACL)
-- This migration adds role-based access control for meeting history

-- Create meeting_access_control table
CREATE TABLE meeting_access_control (
    id SERIAL PRIMARY KEY,
    meeting_id VARCHAR(50) NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('owner', 'editor', 'viewer')),
    granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    granted_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(meeting_id, user_id)
);

-- Index for meeting-based queries (most common: get all users with access to a meeting)
CREATE INDEX IF NOT EXISTS idx_meeting_acl_meeting_id ON meeting_access_control(meeting_id);

-- Index for user-based queries (get all meetings a user has access to)
CREATE INDEX IF NOT EXISTS idx_meeting_acl_user_meeting ON meeting_access_control(user_id, meeting_id);

-- Index for role-based filtering
CREATE INDEX IF NOT EXISTS idx_meeting_acl_meeting_role ON meeting_access_control(meeting_id, role);

-- Backfill: Grant viewer access to all existing participants with user_id
-- Note: Creators don't need ACL entries as they are owners by definition via meetings.created_by
INSERT INTO meeting_access_control (meeting_id, user_id, role, granted_by, granted_at)
SELECT
    mp.meeting_id,
    mp.user_id,
    'viewer' as role,
    m.created_by as granted_by,
    NOW() as granted_at
FROM meeting_participants mp
JOIN meetings m ON mp.meeting_id = m.id
WHERE mp.user_id IS NOT NULL
  AND mp.user_id != m.created_by  -- Don't add ACL for creators (they're owners by definition)
ON CONFLICT (meeting_id, user_id) DO NOTHING;
