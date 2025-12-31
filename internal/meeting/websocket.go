package meeting

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/database"
)

const (
	// Audio buffer configuration
	sampleRate    = 16000
	windowSeconds = 8
	bufferSize    = sampleRate * windowSeconds // 128,000 samples

	// ASR and Translation service URLs
	asrBaseURL        = "http://127.0.0.1:8003"
	translationBaseURL = "http://127.0.0.1:8004"
)

// HandleMeetingWebSocket handles WebSocket connections for meeting rooms
func (rm *RoomManager) HandleMeetingWebSocket(conn *websocket.Conn, meetingID string, participantID int, participantName, targetLang string) {
	log.Printf("Meeting WebSocket connected: participant %d (%s) in meeting %s", participantID, participantName, meetingID)

	// Get participant from database to ensure it exists
	dbParticipant, err := database.GetParticipantByID(participantID)
	if err != nil || dbParticipant == nil {
		log.Printf("Invalid participant ID %d: %v", participantID, err)
		conn.Close()
		return
	}

	// Create participant object
	participant := &Participant{
		ID:             participantID,
		Name:           participantName,
		TargetLanguage: targetLang,
		JoinedAt:       time.Now(),
		Connection:     conn,
	}

	// Add participant to room
	rm.AddParticipant(meetingID, participant)

	// Broadcast participant joined
	rm.Broadcast(meetingID, Message{
		Type:          "participant_joined",
		ParticipantID: participantID,
		ParticipantName: participantName,
		TargetLanguage: targetLang,
	})

	// Audio buffer for streaming
	audioBuffer := make([]int16, 0, bufferSize)
	var bufferMu sync.Mutex

	// Cleanup on disconnect
	defer func() {
		rm.RemoveParticipant(meetingID, participantID)
		database.RemoveParticipant(participantID) // Mark as inactive in database
		rm.Broadcast(meetingID, Message{
			Type:            "participant_left",
			ParticipantID:   participantID,
			ParticipantName: participantName,
		})
		log.Printf("Participant %d (%s) disconnected from meeting %s", participantID, participantName, meetingID)
	}()

	// Read audio data from WebSocket
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for participant %d: %v", participantID, err)
			}
			break
		}

		// Handle binary audio data
		if messageType == websocket.BinaryMessage {
			// Convert bytes to int16 samples
			samples := bytesToInt16(data)

			bufferMu.Lock()
			audioBuffer = append(audioBuffer, samples...)

			// Process chunk when buffer is full
			if len(audioBuffer) >= bufferSize {
				chunk := make([]int16, bufferSize)
				copy(chunk, audioBuffer[:bufferSize])
				audioBuffer = audioBuffer[bufferSize:]
				bufferMu.Unlock()

				// Process chunk asynchronously
				go rm.processAudioChunk(meetingID, participantID, participantName, chunk)
			} else {
				bufferMu.Unlock()
			}
		}

		// Handle JSON control messages (future: change language preference)
		if messageType == websocket.TextMessage {
			var controlMsg map[string]interface{}
			if err := json.Unmarshal(data, &controlMsg); err == nil {
				log.Printf("Control message from participant %d: %v", participantID, controlMsg)
				// Handle control messages here (e.g., change language)
			}
		}
	}
}

// processAudioChunk transcribes audio and broadcasts translations
func (rm *RoomManager) processAudioChunk(meetingID string, participantID int, participantName string, audioSamples []int16) {
	// Voice Activity Detection - check if chunk has sufficient audio level
	if !hasVoiceActivity(audioSamples) {
		// Skip silent or very quiet chunks to avoid hallucination
		return
	}

	// Convert audio samples to WAV format
	wavData, err := samplesToWAV(audioSamples, sampleRate)
	if err != nil {
		log.Printf("Error converting to WAV: %v", err)
		return
	}

	// Transcribe audio
	transcription, sourceLang, err := transcribeAudio(wavData)
	if err != nil {
		log.Printf("Error transcribing audio: %v", err)
		rm.Broadcast(meetingID, Message{
			Type:  "error",
			Error: "Failed to transcribe audio",
		})
		return
	}

	if transcription == "" {
		// No speech detected
		return
	}

	log.Printf("Transcribed from participant %d: %s (lang: %s)", participantID, transcription, sourceLang)

	// Get unique target languages from room
	targetLangs := rm.GetUniqueTargetLanguages(meetingID)
	if len(targetLangs) == 0 {
		log.Printf("No target languages found for meeting %s", meetingID)
		return
	}

	// Translate to all target languages in parallel
	translations := translateParallel(transcription, sourceLang, targetLangs)

	// Debug: Log translations
	log.Printf("Translations for '%s' (from %s) to %v: %v", transcription[:min(50, len(transcription))], sourceLang, targetLangs, translations)

	// Broadcast transcription with translations to all participants
	rm.Broadcast(meetingID, Message{
		Type:                 "transcription",
		SpeakerParticipantID: participantID,
		SpeakerName:          participantName,
		OriginalText:         transcription,
		SourceLanguage:       sourceLang,
		Translations:         translations,
		IsFinal:              true,
	})
}

// transcribeAudio sends audio to ASR service and returns transcription + detected language
func transcribeAudio(wavData []byte) (string, string, error) {
	// Send WAV data directly (not multipart) - same pattern as asr.Client
	url := fmt.Sprintf("%s/detect-language", asrBaseURL)
	req, err := http.NewRequest("POST", url, bytes.NewReader(wavData))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "audio/wav")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("ASR service error: %s", string(bodyBytes))
	}

	// Parse response from detect-language endpoint (includes both text and language)
	var result struct {
		Text     string `json:"text"`
		Language string `json:"language"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	return result.Text, result.Language, nil
}

// translateParallel translates text to multiple languages concurrently
func translateParallel(text, sourceLang string, targetLangs []string) map[string]string {
	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, targetLang := range targetLangs {
		wg.Add(1)
		go func(lang string) {
			defer wg.Done()

			// Skip if source and target are the same
			if lang == sourceLang {
				mu.Lock()
				results[lang] = text
				mu.Unlock()
				return
			}

			// Translate
			translation, err := translateText(text, sourceLang, lang)
			if err != nil {
				log.Printf("Error translating to %s: %v", lang, err)
				translation = text // Fallback to original
			}

			mu.Lock()
			results[lang] = translation
			mu.Unlock()
		}(targetLang)
	}

	wg.Wait()
	return results
}

// translateText sends text to translation service
func translateText(text, sourceLang, targetLang string) (string, error) {
	url := fmt.Sprintf("%s/translate", translationBaseURL)

	reqBody := map[string]string{
		"text":        text,
		"source_lang": sourceLang,
		"target_lang": targetLang,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("translation service error: %s", string(bodyBytes))
	}

	var result struct {
		Translation string `json:"translation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.Translation, nil
}

// Helper functions

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// hasVoiceActivity checks if audio chunk has sufficient energy to contain speech
func hasVoiceActivity(samples []int16) bool {
	if len(samples) == 0 {
		return false
	}

	// Calculate RMS (Root Mean Square) energy
	var sum float64
	for _, sample := range samples {
		normalized := float64(sample) / 32768.0 // Normalize to -1.0 to 1.0
		sum += normalized * normalized
	}
	rms := sum / float64(len(samples))
	energy := rms * 1000 // Scale for easier threshold

	// Threshold for voice activity (tune this value based on testing)
	// Lower = more sensitive (may include background noise)
	// Higher = less sensitive (may miss quiet speech)
	const energyThreshold = 0.5

	hasVoice := energy > energyThreshold

	if !hasVoice {
		log.Printf("Skipping chunk - low energy: %.3f (threshold: %.1f)", energy, energyThreshold)
	} else {
		log.Printf("Processing chunk - energy: %.3f", energy)
	}

	return hasVoice
}

// bytesToInt16 converts byte array to int16 samples
func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

// samplesToWAV converts int16 samples to WAV file format
func samplesToWAV(samples []int16, sampleRate int) ([]byte, error) {
	var buf bytes.Buffer

	// WAV header
	numSamples := len(samples)
	dataSize := numSamples * 2 // 2 bytes per sample (int16)
	fileSize := 36 + dataSize

	// RIFF header
	buf.WriteString("RIFF")
	binary.Write(&buf, binary.LittleEndian, uint32(fileSize))
	buf.WriteString("WAVE")

	// fmt chunk
	buf.WriteString("fmt ")
	binary.Write(&buf, binary.LittleEndian, uint32(16))         // Chunk size
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // Audio format (PCM)
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // Number of channels (mono)
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate)) // Sample rate
	binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*2)) // Byte rate
	binary.Write(&buf, binary.LittleEndian, uint16(2))          // Block align
	binary.Write(&buf, binary.LittleEndian, uint16(16))         // Bits per sample

	// data chunk
	buf.WriteString("data")
	binary.Write(&buf, binary.LittleEndian, uint32(dataSize))

	// Write samples manually to ensure correct byte order
	for _, sample := range samples {
		binary.Write(&buf, binary.LittleEndian, sample)
	}

	return buf.Bytes(), nil
}
