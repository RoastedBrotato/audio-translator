package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/asr"
	"realtime-caption-translator/internal/progress"
	"realtime-caption-translator/internal/session"
	"realtime-caption-translator/internal/translate"
	"realtime-caption-translator/internal/tts"
	"realtime-caption-translator/internal/video"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // dev only
}

type videoUploadResponse struct {
	Success       bool    `json:"success"`
	SessionID     string  `json:"sessionId,omitempty"`
	Transcription string  `json:"transcription,omitempty"`
	Translation   string  `json:"translation,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	VideoPath     string  `json:"videoPath,omitempty"`
	DetectedLang  string  `json:"detectedLang,omitempty"`
	Error         string  `json:"error,omitempty"`
}

func handleVideoUpload(w http.ResponseWriter, r *http.Request, processor *video.Processor, asrClient *asr.Client, translator translate.Translator, ttsClient *tts.Client, progressMgr *progress.Manager) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form first (max 500MB)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("Error parsing form: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "Failed to parse upload",
		})
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		log.Printf("Error getting file: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "No video file provided",
		})
		return
	}

	// Generate session ID for progress tracking
	sessionID := fmt.Sprintf("upload_%d", time.Now().UnixNano())

	// Read form values before starting goroutine
	targetLang := r.FormValue("targetLang")
	if targetLang == "" {
		targetLang = "ar" // Default to Arabic
	}

	sourceLang := r.FormValue("sourceLang")
	if sourceLang == "" {
		sourceLang = "en" // Default to English
	}
	autoDetect := sourceLang == "auto" || sourceLang == "detect"

	// Check if user wants translated audio
	generateTTS := r.FormValue("generateTTS") == "true"

	// Check if user wants voice cloning
	cloneVoice := r.FormValue("cloneVoice") == "true"

	// Send initial response with session ID immediately
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(videoUploadResponse{
		Success:   true,
		SessionID: sessionID,
	})

	// Process asynchronously
	go func() {
		defer file.Close()
		tracker := progressMgr.NewTracker(sessionID)

		tracker.Update("upload", 10, fmt.Sprintf("Received %s (%.2f MB)", header.Filename, float64(header.Size)/(1024*1024)))

		log.Printf("Processing video: %s (%.2f MB), target language: %s", header.Filename, float64(header.Size)/(1024*1024), targetLang)

		tracker.Update("saving", 15, "Saving video file...")

		// Save uploaded file temporarily
		tempDir := processor.TempDir
		tempVideoPath := filepath.Join(tempDir, fmt.Sprintf("upload_%d_%s", time.Now().Unix(), header.Filename))
		defer os.Remove(tempVideoPath)

		outFile, err := os.Create(tempVideoPath)
		if err != nil {
			log.Printf("Error creating temp file: %v", err)
			tracker.Error("saving", "Failed to save video", err)
			return
		}

		if _, err := io.Copy(outFile, file); err != nil {
			outFile.Close()
			log.Printf("Error copying file: %v", err)
			tracker.Error("saving", "Failed to save video", err)
			return
		}
		outFile.Close()

		tracker.Update("extraction", 25, "Extracting audio from video...")

		// Extract audio
		log.Println("Extracting audio from video...")
		audioResult, err := processor.ExtractAudio(tempVideoPath)
		if err != nil {
			log.Printf("Error extracting audio: %v", err)
			tracker.Error("extraction", "Failed to extract audio", err)
			return
		}

		log.Printf("Audio extracted: %.2f seconds, %d bytes", audioResult.Duration, len(audioResult.AudioData))
		tracker.Update("extraction", 35, fmt.Sprintf("Audio extracted: %.2f seconds", audioResult.Duration))

		// Auto-detect language if requested
		var detectedLang string
		if autoDetect {
			tracker.Update("detection", 40, "Detecting language...")
			log.Println("Auto-detecting language...")
			detectedLang, err = asrClient.DetectLanguage(audioResult.AudioData)
			if err != nil {
				log.Printf("Error detecting language: %v, defaulting to 'en'", err)
				detectedLang = "en"
				sourceLang = "en" // Update sourceLang for transcription
				tracker.Update("detection", 45, "Language detection failed, using English")
			} else {
				log.Printf("Detected language: %s", detectedLang)
				sourceLang = detectedLang
				tracker.Update("detection", 45, fmt.Sprintf("Detected language: %s", detectedLang))
			}
		}

		// Transcribe audio
		tracker.Update("transcription", 50, "Transcribing audio...")
		log.Println("Transcribing audio...")
		transcription, err := asrClient.TranscribeWAV(audioResult.AudioData, sourceLang)
		if err != nil {
			log.Printf("Error transcribing: %v", err)
			tracker.Error("transcription", "Failed to transcribe audio", err)
			return
		}

		log.Printf("Transcription: %s", transcription)
		tracker.Update("transcription", 60, "Transcription complete")

		// Translate transcription
		tracker.Update("translation", 65, fmt.Sprintf("Translating from %s to %s...", sourceLang, targetLang))
		log.Printf("Translating from %s to %s...", sourceLang, targetLang)
		translation, err := translator.TranslateWithSource(transcription, sourceLang, targetLang)
		if err != nil {
			log.Printf("Error translating: %v", err)
			tracker.Error("translation", "Failed to translate", err)
			return
		}

		log.Printf("Translation: %s", translation)
		tracker.Update("translation", 70, "Translation complete")

		// Generate TTS and replace audio if requested
		var videoPath string
		if generateTTS && translation != "" {
			var ttsAudio []byte
			var err error

			if cloneVoice {
				// Use voice cloning with original audio as reference
				tracker.Update("tts", 75, "Generating TTS with voice cloning...")
				log.Printf("Generating TTS with voice cloning...")
				ttsAudio, err = ttsClient.SynthesizeWithVoice(translation, targetLang, audioResult.AudioData)
				if err != nil {
					log.Printf("Error with voice cloning, falling back to standard TTS: %v", err)
					tracker.Update("tts", 75, "Voice cloning failed, using standard TTS...")
					// Fallback to standard TTS if voice cloning fails
					ttsAudio, err = ttsClient.Synthesize(translation, targetLang)
					if err != nil {
						log.Printf("Error generating TTS: %v", err)
						tracker.Error("tts", "Failed to generate TTS", err)
						return
					}
				}
			} else {
				// Standard TTS without voice cloning
				tracker.Update("tts", 75, "Generating TTS audio...")
				log.Printf("Generating TTS audio for translation...")
				ttsAudio, err = ttsClient.Synthesize(translation, targetLang)
				if err != nil {
					log.Printf("Error generating TTS: %v", err)
					tracker.Error("tts", "Failed to generate TTS", err)
					return
				}
			}

			log.Printf("Generated TTS audio: %d bytes", len(ttsAudio))
			tracker.Update("tts", 85, "TTS generation complete")

			// Replace audio in video
			tracker.Update("processing", 90, "Replacing audio in video...")
			log.Println("Replacing audio in video...")
			outputVideoPath, err := processor.ReplaceAudio(tempVideoPath, ttsAudio)
			if err != nil {
				log.Printf("Error replacing audio: %v", err)
				tracker.Error("processing", "Failed to replace audio", err)
				return
			}

			// Store the path for download (relative to temp dir)
			videoPath = filepath.Base(outputVideoPath)
			log.Printf("Video with translated audio ready: %s", videoPath)
			tracker.Update("processing", 95, "Video processing complete")
		}

		// Send completion with results
		results := map[string]interface{}{
			"transcription": transcription,
			"translation":   translation,
			"duration":      audioResult.Duration,
			"videoPath":     videoPath,
		}
		if detectedLang != "" {
			results["detectedLang"] = detectedLang
		}
		tracker.CompleteWithResults("Video processing completed successfully", results)
		log.Printf("Video processing completed for session %s", sessionID)
	}() // End of goroutine
}

func main() {
	// Check if ffmpeg is installed
	if err := video.CheckFFmpegInstalled(); err != nil {
		log.Printf("Warning: %v - Video upload feature will not work", err)
	}

	// Create temp directory for video processing
	tempDir := "./temp"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}

	srv := session.NewServer(session.Config{
		ASRBaseURL:    "http://127.0.0.1:8003",
		PollInterval:  800 * time.Millisecond,
		WindowSeconds: 8,
		FinalizeAfter: 500 * time.Millisecond, // Reduced from 900ms for faster finalization
	})

	// Create progress manager
	progressMgr := progress.NewManager()

	// Create video processor
	videoProcessor := video.NewProcessor(tempDir)

	// Create ASR client for batch processing
	asrClient := asr.New("http://127.0.0.1:8003")

	// Create translator
	translator := &translate.HTTPTranslator{
		BaseURL: "http://127.0.0.1:8004",
	}

	// Create TTS client
	ttsClient := tts.New("http://127.0.0.1:8005")

	http.Handle("/", http.FileServer(http.Dir("./web")))
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		go srv.HandleConn(conn)
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		handleVideoUpload(w, r, videoProcessor, asrClient, translator, ttsClient, progressMgr)
	})

	// Recording session management
	var (
		recordingMu       sync.Mutex
		recordingSessions = make(map[string]*session.RecordingSession)
	)

	http.HandleFunc("/recording/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID  string `json:"sessionId"`
			SourceLang string `json:"sourceLang"`
			TargetLang string `json:"targetLang"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		// Create recording session
		recSession := session.NewRecordingSession(session.RecordingConfig{
			SessionID:     req.SessionID,
			SourceLang:    req.SourceLang,
			TargetLang:    req.TargetLang,
			ASRClient:     asrClient,
			Translator:    translator,
			ProgressMgr:   progressMgr,
			SampleRate:    16000,
			WindowSeconds: 8,
		})

		recordingMu.Lock()
		recordingSessions[req.SessionID] = recSession
		recordingMu.Unlock()

		log.Printf("Recording session started: %s", req.SessionID)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":   true,
			"sessionId": req.SessionID,
		})
	})

	http.HandleFunc("/recording/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			SessionID string `json:"sessionId"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		recordingMu.Lock()
		recSession, exists := recordingSessions[req.SessionID]
		recordingMu.Unlock()

		if !exists {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		totalChunks, err := recSession.Stop()
		if err != nil {
			http.Error(w, "Failed to stop session", http.StatusInternalServerError)
			return
		}

		log.Printf("Recording session stopped: %s, total chunks: %d", req.SessionID, totalChunks)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":     true,
			"totalChunks": totalChunks,
		})
	})

	http.HandleFunc("/ws/recording/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 4 {
			http.Error(w, "Invalid session ID", http.StatusBadRequest)
			return
		}
		sessionID := pathParts[3]

		recordingMu.Lock()
		recSession, exists := recordingSessions[sessionID]
		recordingMu.Unlock()

		if !exists {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Recording WebSocket upgrade error:", err)
			return
		}

		log.Printf("Recording WebSocket connected: %s", sessionID)
		recSession.HandleWebSocket(conn)

		// Cleanup after session completes
		go func() {
			time.Sleep(5 * time.Minute)
			recordingMu.Lock()
			delete(recordingSessions, sessionID)
			recordingMu.Unlock()
			log.Printf("Recording session cleaned up: %s", sessionID)
		}()
	})

	http.HandleFunc("/ws/progress/", func(w http.ResponseWriter, r *http.Request) {
		// Extract session ID from URL path
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 4 {
			http.Error(w, "Invalid session ID", http.StatusBadRequest)
			return
		}
		sessionID := pathParts[3]

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Progress WebSocket upgrade error:", err)
			return
		}
		defer conn.Close()

		progressMgr.Subscribe(sessionID, conn)
		defer progressMgr.Unsubscribe(sessionID, conn)

		log.Printf("Progress WebSocket connected for session: %s", sessionID)

		// Keep connection alive and wait for messages
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Progress WebSocket read error: %v", err)
				break
			}
		}
	})

	http.HandleFunc("/download/", func(w http.ResponseWriter, r *http.Request) {
		filename := filepath.Base(r.URL.Path)
		filePath := filepath.Join(tempDir, filename)

		// Security check: ensure file exists and is in temp dir
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
		http.ServeFile(w, r, filePath)

		// Cleanup after download
		go func() {
			time.Sleep(30 * time.Second)
			os.Remove(filePath)
		}()
	})

	// Streaming WebSocket - proxy to ASR streaming service
	http.HandleFunc("/ws/stream", func(w http.ResponseWriter, r *http.Request) {
		log.Println("Streaming WebSocket connection requested")
		// Note: Clients should connect directly to ws://localhost:8003/stream
		http.Error(w, "Connect to ws://localhost:8003/stream", http.StatusOK)
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
