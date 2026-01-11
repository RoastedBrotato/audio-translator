package rag

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"realtime-caption-translator/internal/database"
	"realtime-caption-translator/internal/embedding"
)

// Processor handles chunking and embedding of meeting transcripts
type Processor struct {
	EmbeddingClient *embedding.Client
}

// NewProcessor creates a new RAG processor
func NewProcessor(embeddingClient *embedding.Client) *Processor {
	return &Processor{
		EmbeddingClient: embeddingClient,
	}
}

// ProcessMeetingTranscript chunks and embeds a meeting transcript
func (p *Processor) ProcessMeetingTranscript(meetingID, language, transcript string) error {
	log.Printf("[RAG] Starting processing for meeting %s (language: %s)", meetingID, language)

	// Step 1: Parse and chunk transcript
	chunks, err := p.chunkTranscript(meetingID, language, transcript)
	if err != nil {
		return fmt.Errorf("failed to chunk transcript: %w", err)
	}

	if len(chunks) == 0 {
		log.Printf("[RAG] No chunks generated for meeting %s (transcript empty or invalid)", meetingID)
		return nil
	}

	log.Printf("[RAG] Generated %d chunks for meeting %s", len(chunks), meetingID)

	// Step 2: Extract text from chunks for embedding
	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = chunk.ChunkText
	}

	// Step 3: Generate embeddings for all chunks in batch mode (more efficient)
	embeddings, err := p.EmbeddingClient.EmbedBatch(texts)
	if err != nil {
		log.Printf("[RAG] Failed to generate embeddings for meeting %s: %v", meetingID, err)
		// Mark chunks as failed
		database.UpdateChunkProcessingStatus(meetingID, language, "failed")
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}

	log.Printf("[RAG] Generated %d embeddings for meeting %s", len(embeddings), meetingID)

	// Step 4: Store chunks with embeddings in database
	successCount := 0
	for i, chunk := range chunks {
		chunk.Embedding = embeddings[i]
		chunk.ProcessingStatus = "completed"

		if err := database.CreateMeetingChunk(chunk); err != nil {
			log.Printf("[RAG] Failed to save chunk %d for meeting %s: %v", i, meetingID, err)
			continue
		}
		successCount++
	}

	log.Printf("[RAG] Successfully processed meeting %s: %d/%d chunks saved", meetingID, successCount, len(chunks))

	if successCount == 0 {
		return fmt.Errorf("failed to save any chunks for meeting %s", meetingID)
	}

	return nil
}

// chunkTranscript splits transcript into semantic chunks
// Transcript format: "[HH:MM:SS] SpeakerName: Text\n"
func (p *Processor) chunkTranscript(meetingID, language, transcript string) ([]*database.MeetingChunk, error) {
	lines := strings.Split(transcript, "\n")

	var chunks []*database.MeetingChunk
	var currentChunk strings.Builder
	var chunkStartOffset *float64
	var chunkSpeakers []string
	chunkIndex := 0

	const maxChunkChars = 2000 // ~300 tokens, good for semantic coherence

	// Regex to parse: [HH:MM:SS] SpeakerName: Text
	lineRegex := regexp.MustCompile(`^\[(\d{2}):(\d{2}):(\d{2})\]\s+([^:]+):\s+(.+)$`)

	var lastOffset *float64

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		matches := lineRegex.FindStringSubmatch(line)
		if len(matches) != 6 {
			// Line doesn't match expected format, append to current chunk
			if currentChunk.Len() > 0 {
				currentChunk.WriteString(" ")
			}
			currentChunk.WriteString(line)
			continue
		}

		// Parse timestamp components
		hours := matches[1]
		mins := matches[2]
		secs := matches[3]
		speaker := strings.TrimSpace(matches[4])
		text := strings.TrimSpace(matches[5])

		// Calculate offset in seconds
		var h, m, s int
		fmt.Sscanf(hours, "%d", &h)
		fmt.Sscanf(mins, "%d", &m)
		fmt.Sscanf(secs, "%d", &s)
		offsetSeconds := float64(h*3600 + m*60 + s)
		lastOffset = &offsetSeconds

		// Set chunk start time if this is first entry
		if chunkStartOffset == nil {
			chunkStartOffset = &offsetSeconds
		}

		// Add to current chunk
		if currentChunk.Len() > 0 {
			currentChunk.WriteString(" ")
		}
		currentChunk.WriteString(fmt.Sprintf("%s: %s", speaker, text))

		// Track unique speakers in this chunk
		if !contains(chunkSpeakers, speaker) {
			chunkSpeakers = append(chunkSpeakers, speaker)
		}

		// Check if we should finalize this chunk
		shouldFinalize := false

		// Finalize if chunk exceeds max size
		if currentChunk.Len() > maxChunkChars {
			shouldFinalize = true
		}

		if shouldFinalize && currentChunk.Len() > 0 {
			chunk := p.createChunk(
				meetingID,
				language,
				chunkIndex,
				currentChunk.String(),
				chunkStartOffset,
				&offsetSeconds,
				chunkSpeakers,
			)

			chunks = append(chunks, chunk)
			chunkIndex++

			// Reset for next chunk
			currentChunk.Reset()
			chunkStartOffset = nil
			chunkSpeakers = []string{}
		}
	}

	// Add remaining content as final chunk
	if currentChunk.Len() > 0 {
		chunk := p.createChunk(
			meetingID,
			language,
			chunkIndex,
			currentChunk.String(),
			chunkStartOffset,
			lastOffset,
			chunkSpeakers,
		)
		chunks = append(chunks, chunk)
	}

	return chunks, nil
}

// createChunk creates a MeetingChunk struct from chunk data
func (p *Processor) createChunk(
	meetingID, language string,
	chunkIndex int,
	chunkText string,
	startOffset, endOffset *float64,
	speakers []string,
) *database.MeetingChunk {
	chunk := &database.MeetingChunk{
		MeetingID:          meetingID,
		Language:           language,
		ChunkIndex:         chunkIndex,
		ChunkText:          strings.TrimSpace(chunkText),
		StartOffsetSeconds: startOffset,
		EndOffsetSeconds:   endOffset,
		ProcessingStatus:   "pending",
	}

	// If only one speaker in chunk, add speaker info
	if len(speakers) == 1 {
		speakerName := speakers[0]
		chunk.SpeakerName = &speakerName
	}

	// Calculate timestamps if offsets are available
	if startOffset != nil {
		startTime := time.Unix(int64(*startOffset), 0).UTC()
		chunk.StartTimestamp = &startTime
	}
	if endOffset != nil {
		endTime := time.Unix(int64(*endOffset), 0).UTC()
		chunk.EndTimestamp = &endTime
	}

	return chunk
}

// contains checks if a string slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
