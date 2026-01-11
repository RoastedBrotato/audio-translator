package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// MeetingChunk represents a chunk of meeting transcript with embedding
type MeetingChunk struct {
	ID                 int        `json:"id"`
	MeetingID          string     `json:"meetingId"`
	Language           string     `json:"language"`
	ChunkIndex         int        `json:"chunkIndex"`
	ChunkText          string     `json:"chunkText"`
	SpeakerID          *string    `json:"speakerId,omitempty"`
	SpeakerName        *string    `json:"speakerName,omitempty"`
	StartTimestamp     *time.Time `json:"startTimestamp,omitempty"`
	EndTimestamp       *time.Time `json:"endTimestamp,omitempty"`
	StartOffsetSeconds *float64   `json:"startOffsetSeconds,omitempty"`
	EndOffsetSeconds   *float64   `json:"endOffsetSeconds,omitempty"`
	Embedding          []float32  `json:"-"`
	ProcessingStatus   string     `json:"processingStatus"`
	CreatedAt          time.Time  `json:"createdAt"`
}

// ChatSession represents a RAG conversation session
type ChatSession struct {
	ID           int       `json:"id"`
	SessionID    string    `json:"sessionId"`
	MeetingID    string    `json:"meetingId"`
	Language     string    `json:"language"`
	UserID       *int      `json:"userId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
	LastActivity time.Time `json:"lastActivity"`
}

// ChatMessage represents a message in a RAG conversation
type ChatMessage struct {
	ID              int       `json:"id"`
	SessionID       string    `json:"sessionId"`
	Role            string    `json:"role"` // "user" or "assistant"
	Content         string    `json:"content"`
	ContextChunkIDs []int     `json:"contextChunkIds,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
}

// --- Meeting Chunk operations ---

// CreateMeetingChunk inserts a new chunk with its embedding
func CreateMeetingChunk(chunk *MeetingChunk) error {
	query := `
		INSERT INTO meeting_chunks (
			meeting_id, language, chunk_index, chunk_text,
			speaker_id, speaker_name, start_timestamp, end_timestamp,
			start_offset_seconds, end_offset_seconds, embedding, processing_status
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id, created_at
	`

	// Convert embedding slice to pgvector format string
	embeddingStr := embeddingToString(chunk.Embedding)

	err := DB.QueryRow(
		query,
		chunk.MeetingID,
		chunk.Language,
		chunk.ChunkIndex,
		chunk.ChunkText,
		chunk.SpeakerID,
		chunk.SpeakerName,
		chunk.StartTimestamp,
		chunk.EndTimestamp,
		chunk.StartOffsetSeconds,
		chunk.EndOffsetSeconds,
		embeddingStr,
		chunk.ProcessingStatus,
	).Scan(&chunk.ID, &chunk.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to create meeting chunk: %w", err)
	}

	return nil
}

// SearchSimilarChunks finds top-k most similar chunks using cosine similarity
func SearchSimilarChunks(meetingID, language string, queryEmbedding []float32, topK int) ([]MeetingChunk, error) {
	query := `
		SELECT
			id, meeting_id, language, chunk_index, chunk_text,
			speaker_id, speaker_name, start_timestamp, end_timestamp,
			start_offset_seconds, end_offset_seconds, processing_status, created_at,
			1 - (embedding <=> $1::vector) as similarity
		FROM meeting_chunks
		WHERE meeting_id = $2 AND language = $3 AND processing_status = 'completed'
		ORDER BY embedding <=> $1::vector
		LIMIT $4
	`

	embeddingStr := embeddingToString(queryEmbedding)

	rows, err := DB.Query(query, embeddingStr, meetingID, language, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar chunks: %w", err)
	}
	defer rows.Close()

	var chunks []MeetingChunk
	for rows.Next() {
		var chunk MeetingChunk
		var similarity float64
		var speakerID, speakerName sql.NullString
		var startTimestamp, endTimestamp sql.NullTime
		var startOffset, endOffset sql.NullFloat64

		err := rows.Scan(
			&chunk.ID,
			&chunk.MeetingID,
			&chunk.Language,
			&chunk.ChunkIndex,
			&chunk.ChunkText,
			&speakerID,
			&speakerName,
			&startTimestamp,
			&endTimestamp,
			&startOffset,
			&endOffset,
			&chunk.ProcessingStatus,
			&chunk.CreatedAt,
			&similarity,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		// Handle nullable fields
		if speakerID.Valid {
			chunk.SpeakerID = &speakerID.String
		}
		if speakerName.Valid {
			chunk.SpeakerName = &speakerName.String
		}
		if startTimestamp.Valid {
			chunk.StartTimestamp = &startTimestamp.Time
		}
		if endTimestamp.Valid {
			chunk.EndTimestamp = &endTimestamp.Time
		}
		if startOffset.Valid {
			chunk.StartOffsetSeconds = &startOffset.Float64
		}
		if endOffset.Valid {
			chunk.EndOffsetSeconds = &endOffset.Float64
		}

		chunks = append(chunks, chunk)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating chunks: %w", err)
	}

	return chunks, nil
}

// UpdateChunkProcessingStatus updates the processing status of chunks
func UpdateChunkProcessingStatus(meetingID, language, status string) error {
	query := `
		UPDATE meeting_chunks
		SET processing_status = $1
		WHERE meeting_id = $2 AND language = $3
	`

	_, err := DB.Exec(query, status, meetingID, language)
	if err != nil {
		return fmt.Errorf("failed to update chunk processing status: %w", err)
	}

	return nil
}

// GetChunksByMeeting retrieves all chunks for a meeting
func GetChunksByMeeting(meetingID, language string) ([]MeetingChunk, error) {
	query := `
		SELECT
			id, meeting_id, language, chunk_index, chunk_text,
			speaker_id, speaker_name, start_timestamp, end_timestamp,
			start_offset_seconds, end_offset_seconds, processing_status, created_at
		FROM meeting_chunks
		WHERE meeting_id = $1 AND language = $2
		ORDER BY chunk_index
	`

	rows, err := DB.Query(query, meetingID, language)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunks: %w", err)
	}
	defer rows.Close()

	var chunks []MeetingChunk
	for rows.Next() {
		var chunk MeetingChunk
		var speakerID, speakerName sql.NullString
		var startTimestamp, endTimestamp sql.NullTime
		var startOffset, endOffset sql.NullFloat64

		err := rows.Scan(
			&chunk.ID,
			&chunk.MeetingID,
			&chunk.Language,
			&chunk.ChunkIndex,
			&chunk.ChunkText,
			&speakerID,
			&speakerName,
			&startTimestamp,
			&endTimestamp,
			&startOffset,
			&endOffset,
			&chunk.ProcessingStatus,
			&chunk.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}

		// Handle nullable fields
		if speakerID.Valid {
			chunk.SpeakerID = &speakerID.String
		}
		if speakerName.Valid {
			chunk.SpeakerName = &speakerName.String
		}
		if startTimestamp.Valid {
			chunk.StartTimestamp = &startTimestamp.Time
		}
		if endTimestamp.Valid {
			chunk.EndTimestamp = &endTimestamp.Time
		}
		if startOffset.Valid {
			chunk.StartOffsetSeconds = &startOffset.Float64
		}
		if endOffset.Valid {
			chunk.EndOffsetSeconds = &endOffset.Float64
		}

		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// --- Chat Session operations ---

// CreateChatSession creates a new chat session
func CreateChatSession(meetingID, language string, userID *int) (*ChatSession, error) {
	sessionID := fmt.Sprintf("CHAT_%d", time.Now().UnixNano())

	query := `
		INSERT INTO meeting_chat_sessions (session_id, meeting_id, language, user_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, session_id, meeting_id, language, user_id, created_at, last_activity
	`

	var session ChatSession
	var userIDVal sql.NullInt64

	err := DB.QueryRow(query, sessionID, meetingID, language, userID).Scan(
		&session.ID,
		&session.SessionID,
		&session.MeetingID,
		&session.Language,
		&userIDVal,
		&session.CreatedAt,
		&session.LastActivity,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create chat session: %w", err)
	}

	if userIDVal.Valid {
		uid := int(userIDVal.Int64)
		session.UserID = &uid
	}

	return &session, nil
}

// GetChatSession retrieves a chat session by session ID
func GetChatSession(sessionID string) (*ChatSession, error) {
	query := `
		SELECT id, session_id, meeting_id, language, user_id, created_at, last_activity
		FROM meeting_chat_sessions
		WHERE session_id = $1
	`

	var session ChatSession
	var userID sql.NullInt64

	err := DB.QueryRow(query, sessionID).Scan(
		&session.ID,
		&session.SessionID,
		&session.MeetingID,
		&session.Language,
		&userID,
		&session.CreatedAt,
		&session.LastActivity,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("chat session not found")
		}
		return nil, fmt.Errorf("failed to get chat session: %w", err)
	}

	if userID.Valid {
		uid := int(userID.Int64)
		session.UserID = &uid
	}

	return &session, nil
}

// UpdateChatSessionActivity updates the last activity time for a chat session
func UpdateChatSessionActivity(sessionID string) error {
	query := `
		UPDATE meeting_chat_sessions
		SET last_activity = NOW()
		WHERE session_id = $1
	`

	_, err := DB.Exec(query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to update chat session activity: %w", err)
	}

	return nil
}

// --- Chat Message operations ---

// SaveChatMessage saves a chat message
func SaveChatMessage(msg *ChatMessage) error {
	query := `
		INSERT INTO meeting_chat_messages (session_id, role, content, context_chunk_ids)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`

	err := DB.QueryRow(
		query,
		msg.SessionID,
		msg.Role,
		msg.Content,
		pq.Array(msg.ContextChunkIDs),
	).Scan(&msg.ID, &msg.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to save chat message: %w", err)
	}

	return nil
}

// GetChatHistory retrieves chat history for a session
func GetChatHistory(sessionID string, limit int) ([]ChatMessage, error) {
	query := `
		SELECT id, session_id, role, content, context_chunk_ids, created_at
		FROM meeting_chat_messages
		WHERE session_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := DB.Query(query, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat history: %w", err)
	}
	defer rows.Close()

	var messages []ChatMessage
	for rows.Next() {
		var msg ChatMessage
		err := rows.Scan(
			&msg.ID,
			&msg.SessionID,
			&msg.Role,
			&msg.Content,
			pq.Array(&msg.ContextChunkIDs),
			&msg.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}
		messages = append(messages, msg)
	}

	// Reverse to get chronological order (oldest first)
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// --- Helper functions ---

// embeddingToString converts a float32 slice to pgvector format string
// Format: [1.0, 2.0, 3.0]
func embeddingToString(embedding []float32) string {
	if len(embedding) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteString("[")

	for i, val := range embedding {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("%v", val))
	}

	sb.WriteString("]")
	return sb.String()
}
