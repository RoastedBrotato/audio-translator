package rag

import (
	"fmt"
	"log"
	"strings"

	"realtime-caption-translator/internal/database"
	"realtime-caption-translator/internal/embedding"
	"realtime-caption-translator/internal/llm"
)

// QueryEngine handles RAG queries: retrieve context + generate answers
type QueryEngine struct {
	EmbeddingClient *embedding.Client
	LLMClient       *llm.Client
}

// NewQueryEngine creates a new RAG query engine
func NewQueryEngine(embeddingClient *embedding.Client, llmClient *llm.Client) *QueryEngine {
	return &QueryEngine{
		EmbeddingClient: embeddingClient,
		LLMClient:       llmClient,
	}
}

// Query performs RAG query: retrieve relevant chunks and generate answer (default English)
func (q *QueryEngine) Query(meetingID, language, question string, topK int) (string, []int, error) {
	return q.QueryWithLanguage(meetingID, language, "en", question, topK)
}

// QueryWithLanguage performs RAG query with specified response language
func (q *QueryEngine) QueryWithLanguage(meetingID, transcriptLanguage, chatLanguage, question string, topK int) (string, []int, error) {
	log.Printf("[RAG Query] Processing question for meeting %s (transcript: %s, response: %s)", meetingID, transcriptLanguage, chatLanguage)

	// Step 1: Generate embedding for the question
	questionEmbedding, err := q.EmbeddingClient.Embed(question)
	if err != nil {
		return "", nil, fmt.Errorf("failed to embed question: %w", err)
	}

	log.Printf("[RAG Query] Generated question embedding (%d dims)", len(questionEmbedding))

	// Step 2: Retrieve top-k similar chunks using vector similarity search
	chunks, err := database.SearchSimilarChunks(meetingID, transcriptLanguage, questionEmbedding, topK)
	if err != nil {
		return "", nil, fmt.Errorf("failed to search chunks: %w", err)
	}

	if len(chunks) == 0 {
		log.Printf("[RAG Query] No chunks found for meeting %s", meetingID)
		return "No relevant information found in the meeting transcript. The meeting may not have been processed yet or the transcript may be empty.", nil, nil
	}

	log.Printf("[RAG Query] Retrieved %d relevant chunks", len(chunks))

	// Step 3: Build context from retrieved chunks
	context := q.buildContext(chunks)

	log.Printf("[RAG Query] Built context (%d chars)", len(context))

	// Step 4: Generate answer using LLM with specified chat language
	answer, err := q.LLMClient.GenerateWithLanguage(question, context, chatLanguage, 500, 0.7)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate answer: %w", err)
	}

	log.Printf("[RAG Query] Generated answer (%d chars)", len(answer))

	// Collect chunk IDs for citation
	chunkIDs := make([]int, len(chunks))
	for i, chunk := range chunks {
		chunkIDs[i] = chunk.ID
	}

	return answer, chunkIDs, nil
}

// buildContext creates a formatted context string from retrieved chunks
func (q *QueryEngine) buildContext(chunks []database.MeetingChunk) string {
	var builder strings.Builder

	builder.WriteString("Meeting Transcript Excerpts:\n\n")

	for i, chunk := range chunks {
		builder.WriteString(fmt.Sprintf("--- Excerpt %d ---\n", i+1))

		// Add speaker information if available
		if chunk.SpeakerName != nil {
			builder.WriteString(fmt.Sprintf("Speaker: %s\n", *chunk.SpeakerName))
		}

		// Add timestamp information if available
		if chunk.StartOffsetSeconds != nil {
			mins := int(*chunk.StartOffsetSeconds) / 60
			secs := int(*chunk.StartOffsetSeconds) % 60
			builder.WriteString(fmt.Sprintf("Time: %02d:%02d\n", mins, secs))
		}

		// Add the actual content
		builder.WriteString(fmt.Sprintf("Content: %s\n\n", chunk.ChunkText))
	}

	return builder.String()
}

// QueryWithHistory performs RAG query with conversation history for context
func (q *QueryEngine) QueryWithHistory(meetingID, language, sessionID, question string, topK int) (string, []int, error) {
	// Get chat history
	history, err := database.GetChatHistory(sessionID, 5) // Last 5 messages
	if err != nil {
		log.Printf("[RAG Query] Warning: Could not retrieve chat history: %v", err)
		// Continue without history
		return q.Query(meetingID, language, question, topK)
	}

	// Build question with conversation context
	var contextualQuestion strings.Builder
	if len(history) > 0 {
		contextualQuestion.WriteString("Conversation history:\n")
		for _, msg := range history {
			contextualQuestion.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
		}
		contextualQuestion.WriteString("\nCurrent question: ")
	}
	contextualQuestion.WriteString(question)

	return q.Query(meetingID, language, contextualQuestion.String(), topK)
}
