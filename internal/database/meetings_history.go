package database

import (
	"database/sql"
	"fmt"
	"time"
)

// MeetingHistoryItem represents a meeting in the user's history list
type MeetingHistoryItem struct {
	ID                 string     `json:"id"`
	RoomCode           string     `json:"roomCode"`
	Mode               string     `json:"mode"`
	Role               string     `json:"role"` // "creator" or "participant"
	CreatedAt          time.Time  `json:"createdAt"`
	EndedAt            *time.Time `json:"endedAt,omitempty"`
	IsActive           bool       `json:"isActive"`
	ParticipantCount   int        `json:"participantCount"`
	AvailableLanguages []string   `json:"availableLanguages"`
	DurationSeconds    *int       `json:"durationSeconds,omitempty"`
	MinutesSummary     *string    `json:"minutesSummary,omitempty"`
}

// MeetingDetail represents detailed meeting information
type MeetingDetail struct {
	ID                  string                    `json:"id"`
	RoomCode            string                    `json:"roomCode"`
	Mode                string                    `json:"mode"`
	CreatedAt           time.Time                 `json:"createdAt"`
	EndedAt             *time.Time                `json:"endedAt,omitempty"`
	IsActive            bool                      `json:"isActive"`
	Participants        []MeetingParticipantInfo  `json:"participants"`
	TranscriptSnapshots []TranscriptSnapshotInfo  `json:"transcriptSnapshots"`
	HasRAGChunks        bool                      `json:"hasRAGChunks"`
	ChunkCount          int                       `json:"chunkCount"`
	Minutes             *MeetingMinutesContent    `json:"minutes,omitempty"`
	MinutesSummary      *string                   `json:"minutesSummary,omitempty"`
}

// MeetingParticipantInfo represents participant info for meeting detail
type MeetingParticipantInfo struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	TargetLanguage string     `json:"targetLanguage"`
	JoinedAt       time.Time  `json:"joinedAt"`
	LeftAt         *time.Time `json:"leftAt,omitempty"`
}

// TranscriptSnapshotInfo represents available transcript info
type TranscriptSnapshotInfo struct {
	Language  string    `json:"language"`
	CreatedAt time.Time `json:"createdAt"`
}

// GetUserMeetings returns meetings where user is creator or participant
func GetUserMeetings(userID int, limit, offset int, status string) ([]MeetingHistoryItem, int, error) {
	// Build status filter
	statusFilter := ""
	switch status {
	case "active":
		statusFilter = "AND m.is_active = true"
	case "ended":
		statusFilter = "AND m.is_active = false"
	default:
		// "all" - no filter
	}

	// Main query to get meetings
	query := fmt.Sprintf(`
		SELECT DISTINCT ON (m.id)
			m.id,
			m.room_code,
			m.mode,
			m.created_at,
			m.ended_at,
			m.is_active,
			CASE
				WHEN m.created_by = $1 THEN 'creator'
				ELSE 'participant'
			END as role,
			(SELECT COUNT(*) FROM meeting_participants WHERE meeting_id = m.id) as participant_count,
			CASE
				WHEN m.ended_at IS NOT NULL
				THEN EXTRACT(EPOCH FROM (m.ended_at - m.created_at))::INT
				ELSE NULL
			END as duration_seconds,
			mm.summary as minutes_summary
		FROM meetings m
		LEFT JOIN meeting_participants mp ON mp.meeting_id = m.id AND mp.user_id = $1
		LEFT JOIN meeting_minutes mm ON mm.meeting_id = m.id AND mm.language = 'en'
		WHERE (m.created_by = $1 OR mp.user_id = $1) %s
		ORDER BY m.id, m.created_at DESC
	`, statusFilter)

	// Wrap with ordering and pagination
	paginatedQuery := fmt.Sprintf(`
		SELECT * FROM (%s) sub
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, query)

	rows, err := DB.Query(paginatedQuery, userID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query user meetings: %w", err)
	}
	defer rows.Close()

	var meetings []MeetingHistoryItem
	for rows.Next() {
		var item MeetingHistoryItem
		var endedAt sql.NullTime
		var durationSeconds sql.NullInt64
		var minutesSummary sql.NullString

		err := rows.Scan(
			&item.ID,
			&item.RoomCode,
			&item.Mode,
			&item.CreatedAt,
			&endedAt,
			&item.IsActive,
			&item.Role,
			&item.ParticipantCount,
			&durationSeconds,
			&minutesSummary,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan meeting: %w", err)
		}

		if endedAt.Valid {
			item.EndedAt = &endedAt.Time
		}
		if durationSeconds.Valid {
			dur := int(durationSeconds.Int64)
			item.DurationSeconds = &dur
		}
		if minutesSummary.Valid && minutesSummary.String != "" {
			item.MinutesSummary = &minutesSummary.String
		}

		// Get available languages for this meeting
		languages, err := getMeetingAvailableLanguages(item.ID)
		if err != nil {
			// Don't fail the whole query, just log and continue
			item.AvailableLanguages = []string{}
		} else {
			item.AvailableLanguages = languages
		}

		meetings = append(meetings, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating meetings: %w", err)
	}

	// Get total count
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT m.id)
		FROM meetings m
		LEFT JOIN meeting_participants mp ON mp.meeting_id = m.id AND mp.user_id = $1
		WHERE (m.created_by = $1 OR mp.user_id = $1) %s
	`, statusFilter)

	var total int
	err = DB.QueryRow(countQuery, userID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count user meetings: %w", err)
	}

	return meetings, total, nil
}

// getMeetingAvailableLanguages returns languages with available transcript snapshots
func getMeetingAvailableLanguages(meetingID string) ([]string, error) {
	query := `
		SELECT DISTINCT language
		FROM meeting_transcript_snapshots
		WHERE meeting_id = $1
		ORDER BY language
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var languages []string
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			return nil, err
		}
		languages = append(languages, lang)
	}

	return languages, rows.Err()
}

// GetUserMeetingDetail returns detailed meeting info with authorization check
func GetUserMeetingDetail(userID int, meetingID string) (*MeetingDetail, error) {
	// First check if user can access this meeting
	canAccess, err := UserCanAccessMeeting(userID, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to check meeting access: %w", err)
	}
	if !canAccess {
		return nil, fmt.Errorf("unauthorized: user does not have access to this meeting")
	}

	// Get meeting basic info
	query := `
		SELECT id, room_code, mode, created_at, ended_at, is_active
		FROM meetings
		WHERE id = $1
	`

	var detail MeetingDetail
	var endedAt sql.NullTime

	err = DB.QueryRow(query, meetingID).Scan(
		&detail.ID,
		&detail.RoomCode,
		&detail.Mode,
		&detail.CreatedAt,
		&endedAt,
		&detail.IsActive,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("meeting not found")
		}
		return nil, fmt.Errorf("failed to get meeting: %w", err)
	}

	if endedAt.Valid {
		detail.EndedAt = &endedAt.Time
	}

	// Get participants
	participants, err := getMeetingParticipantsInfo(meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}
	detail.Participants = participants

	// Get transcript snapshots info
	snapshots, err := getMeetingTranscriptSnapshotsInfo(meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transcript snapshots: %w", err)
	}
	detail.TranscriptSnapshots = snapshots

	// Get RAG chunk count
	chunkCount, err := GetMeetingChunkCount(meetingID)
	if err != nil {
		// Don't fail, just set to 0
		chunkCount = 0
	}
	detail.ChunkCount = chunkCount
	detail.HasRAGChunks = chunkCount > 0

	// Get meeting minutes (English)
	minutes, err := GetMeetingMinutes(meetingID, "en")
	if err != nil {
		// Don't fail, just ignore minutes
		minutes = nil
	}
	if minutes != nil {
		detail.Minutes = &minutes.Content
		if minutes.Summary != "" {
			detail.MinutesSummary = &minutes.Summary
		}
	}

	return &detail, nil
}

// getMeetingParticipantsInfo returns all participants for a meeting
func getMeetingParticipantsInfo(meetingID string) ([]MeetingParticipantInfo, error) {
	query := `
		SELECT id, participant_name, target_language, joined_at, left_at
		FROM meeting_participants
		WHERE meeting_id = $1
		ORDER BY joined_at ASC
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []MeetingParticipantInfo
	for rows.Next() {
		var p MeetingParticipantInfo
		var leftAt sql.NullTime

		err := rows.Scan(&p.ID, &p.Name, &p.TargetLanguage, &p.JoinedAt, &leftAt)
		if err != nil {
			return nil, err
		}

		if leftAt.Valid {
			p.LeftAt = &leftAt.Time
		}

		participants = append(participants, p)
	}

	return participants, rows.Err()
}

// getMeetingTranscriptSnapshotsInfo returns available transcript snapshots
func getMeetingTranscriptSnapshotsInfo(meetingID string) ([]TranscriptSnapshotInfo, error) {
	query := `
		SELECT language, created_at
		FROM meeting_transcript_snapshots
		WHERE meeting_id = $1
		ORDER BY language
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snapshots []TranscriptSnapshotInfo
	for rows.Next() {
		var s TranscriptSnapshotInfo
		err := rows.Scan(&s.Language, &s.CreatedAt)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}

	return snapshots, rows.Err()
}

// UserCanAccessMeeting checks if user created or participated in meeting
func UserCanAccessMeeting(userID int, meetingID string) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM meetings WHERE id = $2 AND created_by = $1
			UNION
			SELECT 1 FROM meeting_participants WHERE meeting_id = $2 AND user_id = $1
		)
	`

	var exists bool
	err := DB.QueryRow(query, userID, meetingID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check meeting access: %w", err)
	}

	return exists, nil
}

// GetMeetingChunkCount returns count of RAG chunks for a meeting
func GetMeetingChunkCount(meetingID string) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM meeting_chunks
		WHERE meeting_id = $1 AND processing_status = 'completed'
	`

	var count int
	err := DB.QueryRow(query, meetingID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count meeting chunks: %w", err)
	}

	return count, nil
}
