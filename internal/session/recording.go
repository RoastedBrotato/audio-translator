package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/asr"
	"realtime-caption-translator/internal/audio"
	"realtime-caption-translator/internal/progress"
	"realtime-caption-translator/internal/translate"
)

// RecordingSession handles audio recording with async transcription and translation
type RecordingSession struct {
	ID         string
	SourceLang string
	TargetLang string
	SampleRate int
	WindowSize int // samples per chunk

	asrClient   *asr.Client
	translator  translate.Translator
	progressMgr *progress.Manager

	mu           sync.Mutex
	isRecording  bool
	isStopped    bool
	ring         *audio.Ring
	chunks       [][]int16 // queued audio chunks
	results      []TranscriptItem
	processedIdx int
	totalChunks  int

	wg sync.WaitGroup
}

// TranscriptItem represents a processed audio segment
type TranscriptItem struct {
	Index       int       `json:"index"`
	Original    string    `json:"original"`
	Translation string    `json:"translation"`
	Timestamp   time.Time `json:"timestamp"`
}

// RecordingConfig for creating new recording sessions
type RecordingConfig struct {
	SessionID     string
	SourceLang    string
	TargetLang    string
	ASRClient     *asr.Client
	Translator    translate.Translator
	ProgressMgr   *progress.Manager
	SampleRate    int
	WindowSeconds int
}

// NewRecordingSession creates a new recording session
func NewRecordingSession(cfg RecordingConfig) *RecordingSession {
	windowSize := cfg.SampleRate * cfg.WindowSeconds

	return &RecordingSession{
		ID:          cfg.SessionID,
		SourceLang:  cfg.SourceLang,
		TargetLang:  cfg.TargetLang,
		SampleRate:  cfg.SampleRate,
		WindowSize:  windowSize,
		asrClient:   cfg.ASRClient,
		translator:  cfg.Translator,
		progressMgr: cfg.ProgressMgr,
		ring:        audio.NewRing(windowSize),
		chunks:      make([][]int16, 0),
		results:     make([]TranscriptItem, 0),
	}
}

// HandleWebSocket handles the WebSocket connection for live audio streaming
func (rs *RecordingSession) HandleWebSocket(conn *websocket.Conn) {
	defer conn.Close()

	rs.mu.Lock()
	rs.isRecording = true
	rs.mu.Unlock()

	log.Printf("[Recording %s] WebSocket connected", rs.ID)

	// Start async processor
	rs.wg.Add(1)
	go rs.processQueue(conn)

	// Read audio data from WebSocket
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[Recording %s] WebSocket read error: %v", rs.ID, err)
			break
		}

		if len(data) == 0 {
			continue
		}

		// Convert bytes to int16 PCM
		pcm := make([]int16, len(data)/2)
		for i := 0; i < len(pcm); i++ {
			pcm[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
		}

		// Add to ring buffer
		rs.mu.Lock()
		for _, sample := range pcm {
			rs.ring.Write([]int16{sample})
		}

		// Check if we have a complete chunk
		available := rs.ring.ReadLast(rs.WindowSize)
		if len(available) >= rs.WindowSize {
			chunk := make([]int16, len(available))
			copy(chunk, available)
			rs.chunks = append(rs.chunks, chunk)
			log.Printf("[Recording %s] Queued chunk %d (%d samples)", rs.ID, len(rs.chunks), len(chunk))
			// Reset ring for next chunk
			rs.ring = audio.NewRing(rs.WindowSize)
		}
		rs.mu.Unlock()
	}

	// Connection closed, finalize recording
	rs.mu.Lock()
	rs.isRecording = false

	// Add final partial chunk if any
	finalChunk := rs.ring.ReadLast(rs.WindowSize)
	if len(finalChunk) > 0 {
		chunk := make([]int16, len(finalChunk))
		copy(chunk, finalChunk)
		rs.chunks = append(rs.chunks, chunk)
		log.Printf("[Recording %s] Added final chunk %d (%d samples)", rs.ID, len(rs.chunks), len(chunk))
	}

	rs.totalChunks = len(rs.chunks)
	rs.mu.Unlock()

	log.Printf("[Recording %s] Recording stopped, total chunks: %d", rs.ID, rs.totalChunks)

	// Wait for processing to complete
	rs.wg.Wait()

	// Send completion message via WebSocket if still connected
	completionMsg := map[string]interface{}{
		"type":    "complete",
		"message": "All translations complete",
	}
	if err := conn.WriteJSON(completionMsg); err != nil {
		log.Printf("[Recording %s] Failed to send completion message via WS: %v", rs.ID, err)
	} else {
		log.Printf("[Recording %s] Sent completion message via WebSocket", rs.ID)
	}

	// Send completion message via progress tracker
	if rs.progressMgr != nil {
		rs.progressMgr.SendUpdate(progress.Update{
			SessionID: rs.ID,
			Stage:     "complete",
			Progress:  100,
			Message:   "Recording complete",
		})
		log.Printf("[Recording %s] Sent completion message via progress manager", rs.ID)
	}

	log.Printf("[Recording %s] Processing complete", rs.ID)
}

// processQueue continuously processes queued audio chunks
func (rs *RecordingSession) processQueue(conn *websocket.Conn) {
	defer rs.wg.Done()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		<-ticker.C

		rs.mu.Lock()
		if len(rs.chunks) == 0 {
			// Check if recording is stopped and all chunks processed
			// Exit when: recording stopped, no chunks in queue, and either:
			// - totalChunks is set and we've processed them all, OR
			// - totalChunks is 0 (no chunks were ever created)
			if !rs.isRecording {
				if rs.totalChunks > 0 && rs.processedIdx >= rs.totalChunks {
					// All chunks accounted for and processed
					rs.mu.Unlock()
					log.Printf("[Recording %s] All chunks processed (%d/%d), exiting", rs.ID, rs.processedIdx, rs.totalChunks)
					return
				} else if rs.totalChunks > 0 {
					// totalChunks set but not all processed yet, keep waiting
					rs.mu.Unlock()
					continue
				}
				// totalChunks not set yet, but recording stopped and queue empty
				// Give it one more cycle to let HandleWebSocket set totalChunks
				if rs.processedIdx > 0 {
					rs.mu.Unlock()
					continue
				}
			}
			rs.mu.Unlock()
			continue
		}

		// Get next chunk to process
		chunk := rs.chunks[0]
		rs.chunks = rs.chunks[1:]
		currentIdx := rs.processedIdx + 1
		rs.mu.Unlock()

		// Process this chunk (transcribe + translate)
		rs.processChunk(chunk, currentIdx, conn)

		rs.mu.Lock()
		rs.processedIdx = currentIdx

		// Update progress via tracker
		// Calculate total as max of totalChunks (if recording stopped) or current queue size
		total := rs.totalChunks
		if total == 0 {
			// Still recording or just stopped, estimate total
			total = rs.processedIdx + len(rs.chunks)
		}

		if rs.progressMgr != nil && total > 0 {
			progressPercent := float64(rs.processedIdx*100) / float64(total)
			rs.progressMgr.SendUpdate(progress.Update{
				SessionID: rs.ID,
				Stage:     "processing",
				Progress:  progressPercent,
				Message:   fmt.Sprintf("Processing chunk %d/%d", rs.processedIdx, total),
			})
		}
		rs.mu.Unlock()
	}
}

// processChunk transcribes and translates a single audio chunk
func (rs *RecordingSession) processChunk(pcm []int16, index int, conn *websocket.Conn) {
	log.Printf("[Recording %s] Processing chunk %d (%d samples)", rs.ID, index, len(pcm))

	// Check if audio has sufficient volume (RMS check)
	var sum float64
	for _, sample := range pcm {
		val := float64(sample) / 32768.0
		sum += val * val
	}
	rms := math.Sqrt(sum / float64(len(pcm)))
	log.Printf("[Recording %s] Chunk %d RMS: %.6f", rs.ID, index, rms)

	if rms < 0.01 {
		log.Printf("[Recording %s] Chunk %d too quiet (RMS %.6f), skipping", rs.ID, index, rms)
		return
	}

	// Convert to WAV bytes
	wavBytes := pcmToWav(pcm, rs.SampleRate)

	// Prepare source language
	sourceLang := rs.SourceLang
	if sourceLang == "auto" || sourceLang == "detect" {
		sourceLang = ""
	}

	// Transcribe using TranscribeWAV method
	transcription, err := rs.asrClient.TranscribeWAV(wavBytes, sourceLang)
	if err != nil {
		log.Printf("[Recording %s] Transcription error for chunk %d: %v", rs.ID, index, err)
		return
	}

	if transcription == "" {
		log.Printf("[Recording %s] Empty transcription for chunk %d", rs.ID, index)
		return
	}

	// Filter out hallucinations (repeated characters)
	if isHallucination(transcription) {
		log.Printf("[Recording %s] Detected hallucination in chunk %d: '%s'", rs.ID, index, transcription)
		// Temporarily allow hallucinations through for debugging
		// return
	}

	// Translate using Translate method (2 params: text, targetLang)
	translation, err := rs.translator.Translate(transcription, rs.TargetLang)
	if err != nil {
		log.Printf("[Recording %s] Translation error for chunk %d: %v", rs.ID, index, err)
		translation = transcription // fallback to original
	}

	// Store result
	item := TranscriptItem{
		Index:       index,
		Original:    transcription,
		Translation: translation,
		Timestamp:   time.Now(),
	}

	rs.mu.Lock()
	rs.results = append(rs.results, item)
	rs.mu.Unlock()

	// Prepare translation message
	msg := map[string]interface{}{
		"type":        "translation",
		"index":       index,
		"original":    transcription,
		"translation": translation,
		"timestamp":   item.Timestamp.Format(time.RFC3339),
	}

	// Send to recording WebSocket if still connected
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[Recording %s] Recording WS closed, cannot send translation: %v", rs.ID, err)
	} else {
		log.Printf("[Recording %s] Sent translation via recording WS", rs.ID)
	}

	// ALSO send via progress manager using Results field
	if rs.progressMgr != nil {
		rs.progressMgr.SendUpdate(progress.Update{
			SessionID: rs.ID,
			Stage:     "translation",
			Progress:  -1, // Not a progress update
			Message:   "",
			Results:   msg, // Use Results field for translation data
		})
		log.Printf("[Recording %s] Sent translation via progress manager", rs.ID)
	}

	log.Printf("[Recording %s] Chunk %d processed: '%s' -> '%s'", rs.ID, index, transcription, translation)
}

// Stop marks the session as stopped
func (rs *RecordingSession) Stop() (int, error) {
	rs.mu.Lock()
	rs.isStopped = true
	rs.mu.Unlock()

	log.Printf("[Recording %s] Stop called", rs.ID)

	// Return current chunk count (may increase as final chunks are added)
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return len(rs.chunks), nil
}

// GetResults returns all processed results
func (rs *RecordingSession) GetResults() []TranscriptItem {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	results := make([]TranscriptItem, len(rs.results))
	copy(results, rs.results)
	return results
}

// GetProgress returns current processing progress
func (rs *RecordingSession) GetProgress() (int, int) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	return rs.processedIdx, rs.totalChunks
}

// pcmToWav converts PCM int16 samples to WAV format
func pcmToWav(pcm []int16, sampleRate int) []byte {
	buf := new(bytes.Buffer)

	// WAV header
	dataSize := len(pcm) * 2
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, int32(36+dataSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, int32(16))           // chunk size
	binary.Write(buf, binary.LittleEndian, int16(1))            // PCM
	binary.Write(buf, binary.LittleEndian, int16(1))            // mono
	binary.Write(buf, binary.LittleEndian, int32(sampleRate))   // sample rate
	binary.Write(buf, binary.LittleEndian, int32(sampleRate*2)) // byte rate
	binary.Write(buf, binary.LittleEndian, int16(2))            // block align
	binary.Write(buf, binary.LittleEndian, int16(16))           // bits per sample

	// data chunk
	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, int32(dataSize))
	binary.Write(buf, binary.LittleEndian, pcm)

	return buf.Bytes()
}

// isHallucination detects if the transcription is a hallucination (repeated characters)
func isHallucination(text string) bool {
	if len(text) == 0 {
		return false
	}

	// Count unique runes
	runes := []rune(text)
	uniqueRunes := make(map[rune]bool)
	for _, r := range runes {
		if r != ' ' && r != '\n' && r != '\t' {
			uniqueRunes[r] = true
		}
	}

	// If less than 3 unique characters and text is long, it's likely a hallucination
	if len(uniqueRunes) < 3 && len(runes) > 10 {
		return true
	}

	return false
}
