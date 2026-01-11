package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// MeetingMinutesContent captures the structured minutes fields.
type MeetingMinutesContent struct {
	Participants []string `json:"participants"`
	KeyPoints    []string `json:"key_points"`
	ActionItems  []string `json:"action_items"`
	Decisions    []string `json:"decisions"`
	Summary      string   `json:"summary"`
}

// MeetingMinutes represents stored meeting minutes for a meeting/language.
type MeetingMinutes struct {
	MeetingID string                `json:"meetingId"`
	Language  string                `json:"language"`
	Content   MeetingMinutesContent `json:"content"`
	Summary   string                `json:"summary"`
	CreatedAt time.Time             `json:"createdAt"`
	UpdatedAt time.Time             `json:"updatedAt"`
}

// SaveMeetingMinutes upserts meeting minutes for a meeting/language.
func SaveMeetingMinutes(meetingID, language string, content MeetingMinutesContent) error {
	if language == "" {
		language = "en"
	}

	payload, err := json.Marshal(content)
	if err != nil {
		return fmt.Errorf("failed to marshal meeting minutes: %w", err)
	}

	summary := content.Summary
	query := `
		INSERT INTO meeting_minutes (meeting_id, language, content, summary)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (meeting_id, language)
		DO UPDATE SET content = EXCLUDED.content, summary = EXCLUDED.summary, updated_at = NOW()
	`

	if _, err := DB.Exec(query, meetingID, language, payload, summary); err != nil {
		return fmt.Errorf("failed to save meeting minutes: %w", err)
	}

	return nil
}

// GetMeetingMinutes returns meeting minutes for a meeting/language.
func GetMeetingMinutes(meetingID, language string) (*MeetingMinutes, error) {
	if language == "" {
		language = "en"
	}

	query := `
		SELECT meeting_id, language, content, summary, created_at, updated_at
		FROM meeting_minutes
		WHERE meeting_id = $1 AND language = $2
	`

	var (
		minutes       MeetingMinutes
		contentBytes  []byte
		createdAt     sql.NullTime
		updatedAt     sql.NullTime
	)

	err := DB.QueryRow(query, meetingID, language).Scan(
		&minutes.MeetingID,
		&minutes.Language,
		&contentBytes,
		&minutes.Summary,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get meeting minutes: %w", err)
	}

	if err := json.Unmarshal(contentBytes, &minutes.Content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal meeting minutes: %w", err)
	}

	if createdAt.Valid {
		minutes.CreatedAt = createdAt.Time
	}
	if updatedAt.Valid {
		minutes.UpdatedAt = updatedAt.Time
	}

	return &minutes, nil
}
