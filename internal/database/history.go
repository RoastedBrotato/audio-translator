package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type UserVideoSessionInput struct {
	SessionID       string
	Filename        string
	Transcription   string
	Translation     string
	VideoPath       string
	AudioPath       string
	TTSPath         string
	SourceLang      string
	TargetLang      string
	DurationSeconds int
	ExpiresAt       *time.Time
}

type UserAudioSessionInput struct {
	SessionID      string
	Filename       string
	Transcription  string
	Translation    string
	AudioPath      string
	SourceLang     string
	TargetLang     string
	HasDiarization bool
	NumSpeakers    int
	Segments       json.RawMessage
}

type UserStreamingSessionInput struct {
	SessionID            string
	SourceLang           string
	TargetLang           string
	TotalChunks          int
	TotalDurationSeconds int
	FinalTranscript      string
	FinalTranslation     string
}

type UserFileInput struct {
	SessionType   string
	SessionID     string
	BucketName    string
	FileKey       string
	ContentHash   string
	Etag          string
	MimeType      string
	FileSizeBytes int64
	AccessedAt    *time.Time
}

type UserFileMatch struct {
	ID        int
	SessionID string
	FileKey   string
	CreatedAt time.Time
}

type UserVideoSessionRecord struct {
	SessionID       string
	Filename        string
	Transcription   string
	Translation     string
	VideoPath       string
	AudioPath       string
	TTSPath         string
	SourceLang      string
	TargetLang      string
	DurationSeconds int
	CreatedAt       time.Time
}

type UserAudioSessionRecord struct {
	SessionID      string
	Filename       string
	Transcription  string
	Translation    string
	AudioPath      string
	SourceLang     string
	TargetLang     string
	HasDiarization bool
	NumSpeakers    int
	Segments       json.RawMessage
	CreatedAt      time.Time
}

func FindUserFileByHash(userID int, sessionType, contentHash string) (*UserFileMatch, error) {
	if strings.TrimSpace(contentHash) == "" {
		return nil, nil
	}

	query := `
		SELECT id, session_id, file_key, created_at
		FROM user_files
		WHERE user_id = $1 AND session_type = $2 AND content_hash = $3
		ORDER BY created_at DESC
		LIMIT 1
	`

	var match UserFileMatch
	err := DB.QueryRow(query, userID, sessionType, contentHash).Scan(
		&match.ID,
		&match.SessionID,
		&match.FileKey,
		&match.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lookup user file hash: %w", err)
	}
	return &match, nil
}

func GetUserVideoSessionBySessionID(userID int, sessionID string) (*UserVideoSessionRecord, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}

	query := `
		SELECT session_id, filename, transcription, translation, video_path, audio_path, tts_path,
		       source_lang, target_lang, duration_seconds, created_at
		FROM user_video_sessions
		WHERE user_id = $1 AND session_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var record UserVideoSessionRecord
	var transcription sql.NullString
	var translation sql.NullString
	var videoPath sql.NullString
	var audioPath sql.NullString
	var ttsPath sql.NullString
	var sourceLang sql.NullString
	var targetLang sql.NullString
	var duration sql.NullInt64

	err := DB.QueryRow(query, userID, sessionID).Scan(
		&record.SessionID,
		&record.Filename,
		&transcription,
		&translation,
		&videoPath,
		&audioPath,
		&ttsPath,
		&sourceLang,
		&targetLang,
		&duration,
		&record.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load video session: %w", err)
	}

	if transcription.Valid {
		record.Transcription = transcription.String
	}
	if translation.Valid {
		record.Translation = translation.String
	}
	if videoPath.Valid {
		record.VideoPath = videoPath.String
	}
	if audioPath.Valid {
		record.AudioPath = audioPath.String
	}
	if ttsPath.Valid {
		record.TTSPath = ttsPath.String
	}
	if sourceLang.Valid {
		record.SourceLang = sourceLang.String
	}
	if targetLang.Valid {
		record.TargetLang = targetLang.String
	}
	if duration.Valid {
		record.DurationSeconds = int(duration.Int64)
	}

	return &record, nil
}

func GetUserAudioSessionBySessionID(userID int, sessionID string) (*UserAudioSessionRecord, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, nil
	}

	query := `
		SELECT session_id, filename, transcription, translation, audio_path, source_lang, target_lang,
		       has_diarization, num_speakers, segments, created_at
		FROM user_audio_sessions
		WHERE user_id = $1 AND session_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var record UserAudioSessionRecord
	var transcription sql.NullString
	var translation sql.NullString
	var audioPath sql.NullString
	var sourceLang sql.NullString
	var targetLang sql.NullString
	var numSpeakers sql.NullInt64
	var segments sql.NullString

	err := DB.QueryRow(query, userID, sessionID).Scan(
		&record.SessionID,
		&record.Filename,
		&transcription,
		&translation,
		&audioPath,
		&sourceLang,
		&targetLang,
		&record.HasDiarization,
		&numSpeakers,
		&segments,
		&record.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load audio session: %w", err)
	}

	if transcription.Valid {
		record.Transcription = transcription.String
	}
	if translation.Valid {
		record.Translation = translation.String
	}
	if audioPath.Valid {
		record.AudioPath = audioPath.String
	}
	if sourceLang.Valid {
		record.SourceLang = sourceLang.String
	}
	if targetLang.Valid {
		record.TargetLang = targetLang.String
	}
	if numSpeakers.Valid {
		record.NumSpeakers = int(numSpeakers.Int64)
	}
	if segments.Valid {
		record.Segments = json.RawMessage(segments.String)
	}

	return &record, nil
}
func CreateUserVideoSession(userID int, input UserVideoSessionInput) (int, error) {
	if strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Filename) == "" {
		return 0, fmt.Errorf("session_id and filename are required")
	}

	var expiresAt sql.NullTime
	if input.ExpiresAt != nil {
		expiresAt = sql.NullTime{Time: *input.ExpiresAt, Valid: true}
	}

	query := `
		INSERT INTO user_video_sessions (
			user_id, session_id, filename, transcription, translation, video_path, audio_path, tts_path,
			source_lang, target_lang, duration_seconds, expires_at
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
		        NULLIF($9, ''), NULLIF($10, ''), NULLIF($11, 0), $12)
		RETURNING id
	`

	var id int
	err := DB.QueryRow(
		query,
		userID,
		input.SessionID,
		input.Filename,
		input.Transcription,
		input.Translation,
		input.VideoPath,
		input.AudioPath,
		input.TTSPath,
		input.SourceLang,
		input.TargetLang,
		input.DurationSeconds,
		expiresAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert video session: %w", err)
	}

	return id, nil
}

func CreateUserAudioSession(userID int, input UserAudioSessionInput) (int, error) {
	if strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.Filename) == "" {
		return 0, fmt.Errorf("session_id and filename are required")
	}

	var segments interface{}
	if len(input.Segments) > 0 {
		segments = input.Segments
	}

	query := `
		INSERT INTO user_audio_sessions (
			user_id, session_id, filename, transcription, translation, audio_path, source_lang, target_lang,
			has_diarization, num_speakers, segments
		)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
		        $9, NULLIF($10, 0), $11)
		RETURNING id
	`

	var id int
	err := DB.QueryRow(
		query,
		userID,
		input.SessionID,
		input.Filename,
		input.Transcription,
		input.Translation,
		input.AudioPath,
		input.SourceLang,
		input.TargetLang,
		input.HasDiarization,
		input.NumSpeakers,
		segments,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert audio session: %w", err)
	}

	return id, nil
}

func CreateUserStreamingSession(userID int, input UserStreamingSessionInput) (int, error) {
	if strings.TrimSpace(input.SessionID) == "" {
		return 0, fmt.Errorf("session_id is required")
	}

	query := `
		INSERT INTO user_streaming_sessions (
			user_id, session_id, source_lang, target_lang, total_chunks, total_duration_seconds,
			final_transcript, final_translation
		)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, 0), NULLIF($6, 0), NULLIF($7, ''), NULLIF($8, ''))
		RETURNING id
	`

	var id int
	err := DB.QueryRow(
		query,
		userID,
		input.SessionID,
		input.SourceLang,
		input.TargetLang,
		input.TotalChunks,
		input.TotalDurationSeconds,
		input.FinalTranscript,
		input.FinalTranslation,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert streaming session: %w", err)
	}

	return id, nil
}

func CreateUserFile(userID *int, input UserFileInput) (int, error) {
	if strings.TrimSpace(input.SessionType) == "" || strings.TrimSpace(input.SessionID) == "" {
		return 0, fmt.Errorf("session_type and session_id are required")
	}
	if strings.TrimSpace(input.BucketName) == "" || strings.TrimSpace(input.FileKey) == "" {
		return 0, fmt.Errorf("bucket_name and file_key are required")
	}

	var userIDValue interface{}
	if userID != nil {
		userIDValue = *userID
	}

	var accessedAt sql.NullTime
	if input.AccessedAt != nil {
		accessedAt = sql.NullTime{Time: *input.AccessedAt, Valid: true}
	}

	query := `
		INSERT INTO user_files (
			user_id, session_type, session_id, bucket_name, file_key, content_hash, etag, mime_type, file_size_bytes, accessed_at
		)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''), NULLIF($9, 0), $10)
		RETURNING id
	`

	var id int
	err := DB.QueryRow(
		query,
		userIDValue,
		input.SessionType,
		input.SessionID,
		input.BucketName,
		input.FileKey,
		input.ContentHash,
		input.Etag,
		input.MimeType,
		input.FileSizeBytes,
		accessedAt,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert user file: %w", err)
	}

	return id, nil
}
