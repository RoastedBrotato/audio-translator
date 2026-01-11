package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// User represents a registered user
type User struct {
	ID                int        `json:"id"`
	Username          string     `json:"username"`
	DisplayName       string     `json:"displayName"`
	PreferredLanguage string     `json:"preferredLanguage"`
	Email             string     `json:"email,omitempty"`
	EmailVerified     bool       `json:"emailVerified"`
	LastLogin         *time.Time `json:"lastLogin,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

// Meeting represents a meeting room
type Meeting struct {
	ID        string     `json:"id"`
	RoomCode  string     `json:"roomCode"`
	Mode      string     `json:"mode"` // "individual" or "shared"
	CreatedBy *int       `json:"createdBy,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	EndedAt   *time.Time `json:"endedAt,omitempty"`
	IsActive  bool       `json:"isActive"`
	HostToken string     `json:"-"`
}

// SpeakerMapping represents a speaker name mapping for shared room mode
type SpeakerMapping struct {
	ID          int       `json:"id"`
	MeetingID   string    `json:"meetingId"`
	SpeakerID   string    `json:"speakerId"`   // e.g., "SPEAKER_00"
	SpeakerName string    `json:"speakerName"` // User-assigned name
	CreatedAt   time.Time `json:"createdAt"`
}

// SpeakerProfile represents a diarization speaker profile for persistence.
type SpeakerProfile struct {
	SessionID string    `json:"sessionId"`
	ProfileID string    `json:"profileId"`
	Embedding []float32 `json:"embedding"`
	Count     int       `json:"count"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// TranscriptSnapshot represents a stored meeting transcript snapshot.
type TranscriptSnapshot struct {
	MeetingID  string    `json:"meetingId"`
	Language   string    `json:"language"`
	Transcript string    `json:"transcript"`
	CreatedAt  time.Time `json:"createdAt"`
}

// MeetingParticipant represents a participant in a meeting
type MeetingParticipant struct {
	ID              int        `json:"id"`
	MeetingID       string     `json:"meetingId"`
	UserID          *int       `json:"userId,omitempty"`
	ParticipantName string     `json:"participantName"`
	TargetLanguage  string     `json:"targetLanguage"`
	JoinedAt        time.Time  `json:"joinedAt"`
	LeftAt          *time.Time `json:"leftAt,omitempty"`
	IsActive        bool       `json:"isActive"`
}

// --- User CRUD operations ---

// CreateUser creates a new user
func CreateUser(username, displayName, preferredLang string) (*User, error) {
	query := `
		INSERT INTO users (username, display_name, preferred_language)
		VALUES ($1, $2, $3)
		RETURNING id, username, display_name, preferred_language, email, email_verified, last_login, created_at
	`

	var user User
	var email sql.NullString
	var lastLogin sql.NullTime
	err := DB.QueryRow(query, username, displayName, preferredLang).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PreferredLanguage,
		&email,
		&user.EmailVerified,
		&lastLogin,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	if email.Valid {
		user.Email = email.String
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(username string) (*User, error) {
	query := `
		SELECT id, username, display_name, preferred_language, email, email_verified, last_login, created_at
		FROM users
		WHERE username = $1
	`

	var user User
	var email sql.NullString
	var lastLogin sql.NullTime
	err := DB.QueryRow(query, username).Scan(
		&user.ID,
		&user.Username,
		&user.DisplayName,
		&user.PreferredLanguage,
		&email,
		&user.EmailVerified,
		&lastLogin,
		&user.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if email.Valid {
		user.Email = email.String
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	return &user, nil
}

// --- Meeting CRUD operations ---

// generateRoomCode generates a random 6-character room code (e.g., "ABC-123")
func generateRoomCode() (string, error) {
	bytes := make([]byte, 4)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	encoded := base64.RawURLEncoding.EncodeToString(bytes)
	code := strings.ToUpper(encoded[:6])

	// Format as ABC-123
	if len(code) >= 6 {
		return fmt.Sprintf("%s-%s", code[:3], code[3:6]), nil
	}
	return code, nil
}

func generateHostToken() (string, error) {
	bytes := make([]byte, 24)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// CreateMeeting creates a new meeting
func CreateMeeting(createdByUserID *int, mode string) (*Meeting, error) {
	// Default to individual mode if not specified
	if mode == "" {
		mode = "individual"
	}

	roomCode, err := generateRoomCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate room code: %w", err)
	}

	meetingID := fmt.Sprintf("MTG_%d", time.Now().UnixNano())
	hostToken, err := generateHostToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate host token: %w", err)
	}

	query := `
		INSERT INTO meetings (id, room_code, mode, created_by, host_token, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
		RETURNING id, room_code, mode, created_by, created_at, ended_at, is_active, host_token
	`

	var meeting Meeting
	err = DB.QueryRow(query, meetingID, roomCode, mode, createdByUserID, hostToken).Scan(
		&meeting.ID,
		&meeting.RoomCode,
		&meeting.Mode,
		&meeting.CreatedBy,
		&meeting.CreatedAt,
		&meeting.EndedAt,
		&meeting.IsActive,
		&meeting.HostToken,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create meeting: %w", err)
	}

	return &meeting, nil
}

// ValidateMeetingHostToken verifies that the host token matches a meeting.
func ValidateMeetingHostToken(meetingID, hostToken string) (bool, error) {
	if meetingID == "" || hostToken == "" {
		return false, nil
	}
	var exists bool
	err := DB.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM meetings WHERE id = $1 AND host_token = $2)`,
		meetingID,
		hostToken,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to validate host token: %w", err)
	}
	return exists, nil
}

// GetMeetingByRoomCode retrieves a meeting by room code
func GetMeetingByRoomCode(roomCode string) (*Meeting, error) {
	query := `
		SELECT id, room_code, mode, created_by, created_at, ended_at, is_active
		FROM meetings
		WHERE room_code = $1
	`

	var meeting Meeting
	err := DB.QueryRow(query, roomCode).Scan(
		&meeting.ID,
		&meeting.RoomCode,
		&meeting.Mode,
		&meeting.CreatedBy,
		&meeting.CreatedAt,
		&meeting.EndedAt,
		&meeting.IsActive,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get meeting: %w", err)
	}

	return &meeting, nil
}

// GetMeetingByID retrieves a meeting by ID
func GetMeetingByID(meetingID string) (*Meeting, error) {
	query := `
		SELECT id, room_code, mode, created_by, created_at, ended_at, is_active
		FROM meetings
		WHERE id = $1
	`

	var meeting Meeting
	err := DB.QueryRow(query, meetingID).Scan(
		&meeting.ID,
		&meeting.RoomCode,
		&meeting.Mode,
		&meeting.CreatedBy,
		&meeting.CreatedAt,
		&meeting.EndedAt,
		&meeting.IsActive,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get meeting: %w", err)
	}

	return &meeting, nil
}

// EndMeeting marks a meeting as ended
func EndMeeting(meetingID string) error {
	query := `
		UPDATE meetings
		SET ended_at = NOW(), is_active = false
		WHERE id = $1
	`

	_, err := DB.Exec(query, meetingID)
	if err != nil {
		return fmt.Errorf("failed to end meeting: %w", err)
	}

	return nil
}

// --- Participant CRUD operations ---

// AddParticipant adds a participant to a meeting
func AddParticipant(meetingID string, userID *int, participantName, targetLang string) (*MeetingParticipant, error) {
	query := `
		INSERT INTO meeting_participants (meeting_id, user_id, participant_name, target_language, is_active)
		VALUES ($1, $2, $3, $4, true)
		RETURNING id, meeting_id, user_id, participant_name, target_language, joined_at, left_at, is_active
	`

	var participant MeetingParticipant
	err := DB.QueryRow(query, meetingID, userID, participantName, targetLang).Scan(
		&participant.ID,
		&participant.MeetingID,
		&participant.UserID,
		&participant.ParticipantName,
		&participant.TargetLanguage,
		&participant.JoinedAt,
		&participant.LeftAt,
		&participant.IsActive,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to add participant: %w", err)
	}

	return &participant, nil
}

// GetActiveParticipants retrieves all active participants in a meeting
func GetActiveParticipants(meetingID string) ([]MeetingParticipant, error) {
	query := `
		SELECT id, meeting_id, user_id, participant_name, target_language, joined_at, left_at, is_active
		FROM meeting_participants
		WHERE meeting_id = $1 AND is_active = true
		ORDER BY joined_at ASC
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}
	defer rows.Close()

	var participants []MeetingParticipant
	for rows.Next() {
		var p MeetingParticipant
		err := rows.Scan(
			&p.ID,
			&p.MeetingID,
			&p.UserID,
			&p.ParticipantName,
			&p.TargetLanguage,
			&p.JoinedAt,
			&p.LeftAt,
			&p.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, p)
	}

	return participants, nil
}

// GetMeetingParticipants retrieves all participants in a meeting.
func GetMeetingParticipants(meetingID string) ([]MeetingParticipant, error) {
	query := `
		SELECT id, meeting_id, user_id, participant_name, target_language, joined_at, left_at, is_active
		FROM meeting_participants
		WHERE meeting_id = $1
		ORDER BY joined_at ASC
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get participants: %w", err)
	}
	defer rows.Close()

	var participants []MeetingParticipant
	for rows.Next() {
		var p MeetingParticipant
		err := rows.Scan(
			&p.ID,
			&p.MeetingID,
			&p.UserID,
			&p.ParticipantName,
			&p.TargetLanguage,
			&p.JoinedAt,
			&p.LeftAt,
			&p.IsActive,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan participant: %w", err)
		}
		participants = append(participants, p)
	}

	return participants, rows.Err()
}

// GetParticipantByID retrieves a participant by ID
func GetParticipantByID(participantID int) (*MeetingParticipant, error) {
	query := `
		SELECT id, meeting_id, user_id, participant_name, target_language, joined_at, left_at, is_active
		FROM meeting_participants
		WHERE id = $1
	`

	var participant MeetingParticipant
	err := DB.QueryRow(query, participantID).Scan(
		&participant.ID,
		&participant.MeetingID,
		&participant.UserID,
		&participant.ParticipantName,
		&participant.TargetLanguage,
		&participant.JoinedAt,
		&participant.LeftAt,
		&participant.IsActive,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get participant: %w", err)
	}

	return &participant, nil
}

// UpdateParticipantLanguage updates a participant's target language
func UpdateParticipantLanguage(participantID int, targetLang string) error {
	query := `
		UPDATE meeting_participants
		SET target_language = $1
		WHERE id = $2
	`

	_, err := DB.Exec(query, targetLang, participantID)
	if err != nil {
		return fmt.Errorf("failed to update participant language: %w", err)
	}

	return nil
}

// UpdateParticipantUserID links a participant record to a user.
func UpdateParticipantUserID(participantID int, userID int) error {
	query := `
		UPDATE meeting_participants
		SET user_id = $1
		WHERE id = $2
	`

	_, err := DB.Exec(query, userID, participantID)
	if err != nil {
		return fmt.Errorf("failed to update participant user: %w", err)
	}

	return nil
}

// RemoveParticipant marks a participant as inactive (left the meeting)
func RemoveParticipant(participantID int) error {
	query := `
		UPDATE meeting_participants
		SET left_at = NOW(), is_active = false
		WHERE id = $1
	`

	_, err := DB.Exec(query, participantID)
	if err != nil {
		return fmt.Errorf("failed to remove participant: %w", err)
	}

	return nil
}

// GetUniqueTargetLanguages retrieves all unique target languages for a meeting
func GetUniqueTargetLanguages(meetingID string) ([]string, error) {
	query := `
		SELECT DISTINCT target_language
		FROM meeting_participants
		WHERE meeting_id = $1 AND is_active = true
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target languages: %w", err)
	}
	defer rows.Close()

	var languages []string
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			return nil, fmt.Errorf("failed to scan language: %w", err)
		}
		languages = append(languages, lang)
	}

	return languages, nil
}

// --- Speaker Mapping CRUD operations (for shared room mode) ---

// SetSpeakerName creates or updates a speaker name mapping
func SetSpeakerName(meetingID, speakerID, speakerName string) error {
	query := `
		INSERT INTO speaker_mappings (meeting_id, speaker_id, speaker_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (meeting_id, speaker_id)
		DO UPDATE SET speaker_name = EXCLUDED.speaker_name
	`

	_, err := DB.Exec(query, meetingID, speakerID, speakerName)
	if err != nil {
		return fmt.Errorf("failed to set speaker name: %w", err)
	}

	return nil
}

// GetSpeakerMappings retrieves all speaker name mappings for a meeting
func GetSpeakerMappings(meetingID string) (map[string]string, error) {
	query := `
		SELECT speaker_id, speaker_name
		FROM speaker_mappings
		WHERE meeting_id = $1
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to get speaker mappings: %w", err)
	}
	defer rows.Close()

	mappings := make(map[string]string)
	for rows.Next() {
		var speakerID, speakerName string
		if err := rows.Scan(&speakerID, &speakerName); err != nil {
			return nil, fmt.Errorf("failed to scan speaker mapping: %w", err)
		}
		mappings[speakerID] = speakerName
	}

	return mappings, nil
}

// GetSpeakerName retrieves the name for a specific speaker
func GetSpeakerName(meetingID, speakerID string) (string, error) {
	query := `
		SELECT speaker_name
		FROM speaker_mappings
		WHERE meeting_id = $1 AND speaker_id = $2
	`

	var speakerName string
	err := DB.QueryRow(query, meetingID, speakerID).Scan(&speakerName)
	if err == sql.ErrNoRows {
		// Return speaker ID as default if no mapping exists
		return speakerID, nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get speaker name: %w", err)
	}

	return speakerName, nil
}

// SaveMeetingTranscriptSnapshot stores the final transcript for a meeting/language
func SaveMeetingTranscriptSnapshot(meetingID, language, transcript string) error {
	if meetingID == "" || language == "" || transcript == "" {
		return fmt.Errorf("meeting transcript snapshot requires meetingID, language, and transcript")
	}

	query := `
		INSERT INTO meeting_transcript_snapshots (meeting_id, language, transcript)
		VALUES ($1, $2, $3)
		ON CONFLICT (meeting_id, language)
		DO UPDATE SET transcript = EXCLUDED.transcript, created_at = NOW()
	`
	_, err := DB.Exec(query, meetingID, language, transcript)
	if err != nil {
		return fmt.Errorf("failed to save meeting transcript snapshot: %w", err)
	}
	return nil
}

// ReplaceSpeakerProfiles replaces stored profiles for a session.
func ReplaceSpeakerProfiles(sessionID string, profiles []SpeakerProfile) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID is required")
	}

	tx, err := DB.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin speaker profile transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM speaker_profiles WHERE session_id = $1`, sessionID); err != nil {
		return fmt.Errorf("failed to clear speaker profiles: %w", err)
	}

	insertQuery := `
		INSERT INTO speaker_profiles (session_id, profile_id, embedding, count, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
	`
	for _, profile := range profiles {
		embeddingJSON, err := json.Marshal(profile.Embedding)
		if err != nil {
			return fmt.Errorf("failed to marshal speaker profile embedding: %w", err)
		}
		if _, err := tx.Exec(insertQuery, sessionID, profile.ProfileID, embeddingJSON, profile.Count); err != nil {
			return fmt.Errorf("failed to insert speaker profile: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit speaker profiles: %w", err)
	}
	return nil
}

// GetSpeakerProfiles retrieves stored speaker profiles for a session.
func GetSpeakerProfiles(sessionID string) ([]SpeakerProfile, error) {
	query := `
		SELECT session_id, profile_id, embedding, count, updated_at
		FROM speaker_profiles
		WHERE session_id = $1
		ORDER BY profile_id
	`

	rows, err := DB.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query speaker profiles: %w", err)
	}
	defer rows.Close()

	var profiles []SpeakerProfile
	for rows.Next() {
		var profile SpeakerProfile
		var embeddingJSON []byte
		if err := rows.Scan(&profile.SessionID, &profile.ProfileID, &embeddingJSON, &profile.Count, &profile.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan speaker profile: %w", err)
		}
		if err := json.Unmarshal(embeddingJSON, &profile.Embedding); err != nil {
			return nil, fmt.Errorf("failed to unmarshal speaker profile embedding: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read speaker profiles: %w", err)
	}

	return profiles, nil
}

// DeleteSpeakerProfiles removes profiles for a session.
func DeleteSpeakerProfiles(sessionID string) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID is required")
	}
	if _, err := DB.Exec(`DELETE FROM speaker_profiles WHERE session_id = $1`, sessionID); err != nil {
		return fmt.Errorf("failed to delete speaker profiles: %w", err)
	}
	return nil
}

// DeleteExpiredSpeakerProfiles removes profiles not updated since cutoff.
func DeleteExpiredSpeakerProfiles(cutoff time.Time) (int64, error) {
	result, err := DB.Exec(`DELETE FROM speaker_profiles WHERE updated_at < $1`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired speaker profiles: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to read delete count: %w", err)
	}
	return rows, nil
}

// GetMeetingTranscriptSnapshot retrieves a transcript snapshot for a meeting/language.
func GetMeetingTranscriptSnapshot(meetingID, language string) (*TranscriptSnapshot, error) {
	query := `
		SELECT meeting_id, language, transcript, created_at
		FROM meeting_transcript_snapshots
		WHERE meeting_id = $1 AND language = $2
	`

	var snapshot TranscriptSnapshot
	err := DB.QueryRow(query, meetingID, language).Scan(
		&snapshot.MeetingID,
		&snapshot.Language,
		&snapshot.Transcript,
		&snapshot.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get meeting transcript snapshot: %w", err)
	}
	return &snapshot, nil
}

// ListMeetingTranscriptSnapshots returns available snapshot languages for a meeting.
func ListMeetingTranscriptSnapshots(meetingID string) ([]TranscriptSnapshot, error) {
	query := `
		SELECT meeting_id, language, created_at
		FROM meeting_transcript_snapshots
		WHERE meeting_id = $1
		ORDER BY language
	`

	rows, err := DB.Query(query, meetingID)
	if err != nil {
		return nil, fmt.Errorf("failed to list meeting transcript snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []TranscriptSnapshot
	for rows.Next() {
		var snapshot TranscriptSnapshot
		if err := rows.Scan(&snapshot.MeetingID, &snapshot.Language, &snapshot.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan meeting transcript snapshot: %w", err)
		}
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to read meeting transcript snapshots: %w", err)
	}
	return snapshots, nil
}
