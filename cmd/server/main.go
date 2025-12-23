package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/asr"
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
	Transcription string  `json:"transcription,omitempty"`
	Translation   string  `json:"translation,omitempty"`
	Duration      float64 `json:"duration,omitempty"`
	VideoPath     string  `json:"videoPath,omitempty"`
	Error         string  `json:"error,omitempty"`
}

func handleVideoUpload(w http.ResponseWriter, r *http.Request, processor *video.Processor, asrClient *asr.Client, translator translate.Translator, ttsClient *tts.Client) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 500MB)
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
	defer file.Close()

	targetLang := r.FormValue("targetLang")
	if targetLang == "" {
		targetLang = "ar" // Default to Arabic
	}

	sourceLang := r.FormValue("sourceLang")
	if sourceLang == "" {
		sourceLang = "en" // Default to English
	}

	// Check if user wants translated audio
	generateTTS := r.FormValue("generateTTS") == "true"

	// Check if user wants voice cloning
	cloneVoice := r.FormValue("cloneVoice") == "true"

	log.Printf("Processing video: %s (%.2f MB), target language: %s", header.Filename, float64(header.Size)/(1024*1024), targetLang)

	// Save uploaded file temporarily
	tempDir := processor.TempDir
	tempVideoPath := filepath.Join(tempDir, fmt.Sprintf("upload_%d_%s", time.Now().Unix(), header.Filename))
	defer os.Remove(tempVideoPath)

	outFile, err := os.Create(tempVideoPath)
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "Failed to save video",
		})
		return
	}

	if _, err := io.Copy(outFile, file); err != nil {
		outFile.Close()
		log.Printf("Error copying file: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "Failed to save video",
		})
		return
	}
	outFile.Close()

	// Extract audio
	log.Println("Extracting audio from video...")
	audioResult, err := processor.ExtractAudio(tempVideoPath)
	if err != nil {
		log.Printf("Error extracting audio: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to extract audio: %v", err),
		})
		return
	}

	log.Printf("Audio extracted: %.2f seconds, %d bytes", audioResult.Duration, len(audioResult.AudioData))

	// Transcribe audio
	log.Println("Transcribing audio...")
	transcription, err := asrClient.TranscribeWAV(audioResult.AudioData, sourceLang)
	if err != nil {
		log.Printf("Error transcribing: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to transcribe audio: %v", err),
		})
		return
	}

	log.Printf("Transcription: %s", transcription)

	// Translate transcription
	log.Printf("Translating to %s...", targetLang)
	translation, err := translator.Translate(transcription, targetLang)
	if err != nil {
		log.Printf("Error translating: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to translate: %v", err),
		})
		return
	}

	log.Printf("Translation: %s", translation)

	// Generate TTS and replace audio if requested
	var videoPath string
	if generateTTS && translation != "" {
		var ttsAudio []byte
		var err error

		if cloneVoice {
			// Use voice cloning with original audio as reference
			log.Printf("Generating TTS with voice cloning...")
			ttsAudio, err = ttsClient.SynthesizeWithVoice(translation, targetLang, audioResult.AudioData)
			if err != nil {
				log.Printf("Error with voice cloning, falling back to standard TTS: %v", err)
				// Fallback to standard TTS if voice cloning fails
				ttsAudio, err = ttsClient.Synthesize(translation, targetLang)
				if err != nil {
					log.Printf("Error generating TTS: %v", err)
					json.NewEncoder(w).Encode(videoUploadResponse{
						Success: false,
						Error:   fmt.Sprintf("Failed to generate TTS: %v", err),
					})
					return
				}
			}
		} else {
			// Standard TTS without voice cloning
			log.Printf("Generating TTS audio for translation...")
			ttsAudio, err = ttsClient.Synthesize(translation, targetLang)
			if err != nil {
				log.Printf("Error generating TTS: %v", err)
				json.NewEncoder(w).Encode(videoUploadResponse{
					Success: false,
					Error:   fmt.Sprintf("Failed to generate TTS: %v", err),
				})
				return
			}
		}

		log.Printf("Generated TTS audio: %d bytes", len(ttsAudio))

		// Replace audio in video
		log.Println("Replacing audio in video...")
		outputVideoPath, err := processor.ReplaceAudio(tempVideoPath, ttsAudio)
		if err != nil {
			log.Printf("Error replacing audio: %v", err)
			json.NewEncoder(w).Encode(videoUploadResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to replace audio: %v", err),
			})
			return
		}

		// Store the path for download (relative to temp dir)
		videoPath = filepath.Base(outputVideoPath)
		log.Printf("Video with translated audio ready: %s", videoPath)
	}

	// Send response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(videoUploadResponse{
		Success:       true,
		Transcription: transcription,
		Translation:   translation,
		Duration:      audioResult.Duration,
		VideoPath:     videoPath,
	})
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
		ASRBaseURL:       "http://127.0.0.1:8003",
		TranslateBaseURL: "http://127.0.0.1:8004",
		PollInterval:     800 * time.Millisecond,
		WindowSeconds:    8,
		FinalizeAfter:    500 * time.Millisecond, // Reduced from 900ms for faster finalization
	})

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
		handleVideoUpload(w, r, videoProcessor, asrClient, translator, ttsClient)
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

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
