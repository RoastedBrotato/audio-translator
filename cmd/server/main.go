package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"realtime-caption-translator/internal/asr"
	"realtime-caption-translator/internal/auth"
	"realtime-caption-translator/internal/database"
	"realtime-caption-translator/internal/embedding"
	"realtime-caption-translator/internal/llm"
	"realtime-caption-translator/internal/meeting"
	"realtime-caption-translator/internal/progress"
	"realtime-caption-translator/internal/rag"
	"realtime-caption-translator/internal/session"
	"realtime-caption-translator/internal/storage"
	"realtime-caption-translator/internal/translate"
	"realtime-caption-translator/internal/tts"
	"realtime-caption-translator/internal/video"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Get allowed origins from environment variable (comma-separated)
		// Example: ALLOWED_ORIGINS=http://localhost:3000,https://yourdomain.com
		allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")

		// For development, allow all origins if not configured
		// In production, you MUST set ALLOWED_ORIGINS
		if allowedOriginsEnv == "" {
			log.Println("WARNING: ALLOWED_ORIGINS not set - allowing all origins (development mode)")
			return true
		}

		origin := r.Header.Get("Origin")
		allowedOrigins := strings.Split(allowedOriginsEnv, ",")

		for _, allowed := range allowedOrigins {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}

		log.Printf("Rejected WebSocket connection from unauthorized origin: %s", origin)
		return false
	},
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

type authResponse struct {
	Success bool           `json:"success"`
	User    *database.User `json:"user,omitempty"`
	Error   string         `json:"error,omitempty"`
}

func handleKeycloakLogin(verifier *auth.KeycloakVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if verifier == nil {
			http.Error(w, "Keycloak auth not configured", http.StatusServiceUnavailable)
			return
		}

		tokenStr, err := extractBearerToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		claims, err := verifier.VerifyToken(r.Context(), tokenStr)
		if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		user, err := upsertUserFromClaims(claims)
		if err != nil {
			log.Printf("Keycloak upsert failed: %v", err)
			http.Error(w, "Failed to persist user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(authResponse{
			Success: true,
			User:    user,
		})
	}
}

type historyVideoRequest struct {
	SessionID       string     `json:"sessionId"`
	Filename        string     `json:"filename"`
	Transcription   string     `json:"transcription,omitempty"`
	Translation     string     `json:"translation,omitempty"`
	VideoPath       string     `json:"videoPath,omitempty"`
	AudioPath       string     `json:"audioPath,omitempty"`
	TTSPath         string     `json:"ttsPath,omitempty"`
	SourceLang      string     `json:"sourceLang,omitempty"`
	TargetLang      string     `json:"targetLang,omitempty"`
	DurationSeconds int        `json:"durationSeconds,omitempty"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
}

type historyAudioRequest struct {
	SessionID      string          `json:"sessionId"`
	Filename       string          `json:"filename"`
	Transcription  string          `json:"transcription,omitempty"`
	Translation    string          `json:"translation,omitempty"`
	AudioPath      string          `json:"audioPath,omitempty"`
	SourceLang     string          `json:"sourceLang,omitempty"`
	TargetLang     string          `json:"targetLang,omitempty"`
	HasDiarization bool            `json:"hasDiarization,omitempty"`
	NumSpeakers    int             `json:"numSpeakers,omitempty"`
	Segments       json.RawMessage `json:"segments,omitempty"`
}

type historyStreamingRequest struct {
	SessionID            string `json:"sessionId"`
	SourceLang           string `json:"sourceLang,omitempty"`
	TargetLang           string `json:"targetLang,omitempty"`
	TotalChunks          int    `json:"totalChunks,omitempty"`
	TotalDurationSeconds int    `json:"totalDurationSeconds,omitempty"`
	FinalTranscript      string `json:"finalTranscript,omitempty"`
	FinalTranslation     string `json:"finalTranslation,omitempty"`
}

type userFileRequest struct {
	SessionType   string     `json:"sessionType"`
	SessionID     string     `json:"sessionId"`
	BucketName    string     `json:"bucketName"`
	FileKey       string     `json:"fileKey"`
	ContentHash   string     `json:"contentHash,omitempty"`
	Etag          string     `json:"etag,omitempty"`
	MimeType      string     `json:"mimeType,omitempty"`
	FileSizeBytes int64      `json:"fileSizeBytes,omitempty"`
	AccessedAt    *time.Time `json:"accessedAt,omitempty"`
}

type historyResponse struct {
	Success bool   `json:"success"`
	ID      int    `json:"id,omitempty"`
	Error   string `json:"error,omitempty"`
}

func handleCreateVideoHistory(verifier *auth.KeycloakVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user, ok := authenticateUserFromRequest(verifier, w, r)
		if !ok {
			return
		}

		var req historyVideoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		id, err := database.CreateUserVideoSession(user.ID, database.UserVideoSessionInput{
			SessionID:       req.SessionID,
			Filename:        req.Filename,
			Transcription:   req.Transcription,
			Translation:     req.Translation,
			VideoPath:       req.VideoPath,
			AudioPath:       req.AudioPath,
			TTSPath:         req.TTSPath,
			SourceLang:      req.SourceLang,
			TargetLang:      req.TargetLang,
			DurationSeconds: req.DurationSeconds,
			ExpiresAt:       req.ExpiresAt,
		})
		if err != nil {
			log.Printf("Create video history failed: %v", err)
			http.Error(w, "Failed to store history", http.StatusInternalServerError)
			return
		}

		writeJSON(w, historyResponse{Success: true, ID: id})
	}
}

func handleCreateAudioHistory(verifier *auth.KeycloakVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user, ok := authenticateUserFromRequest(verifier, w, r)
		if !ok {
			return
		}

		var req historyAudioRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		hasDiarization := req.HasDiarization || len(req.Segments) > 0

		id, err := database.CreateUserAudioSession(user.ID, database.UserAudioSessionInput{
			SessionID:      req.SessionID,
			Filename:       req.Filename,
			Transcription:  req.Transcription,
			Translation:    req.Translation,
			AudioPath:      req.AudioPath,
			SourceLang:     req.SourceLang,
			TargetLang:     req.TargetLang,
			HasDiarization: hasDiarization,
			NumSpeakers:    req.NumSpeakers,
			Segments:       req.Segments,
		})
		if err != nil {
			log.Printf("Create audio history failed: %v", err)
			http.Error(w, "Failed to store history", http.StatusInternalServerError)
			return
		}

		writeJSON(w, historyResponse{Success: true, ID: id})
	}
}

func handleCreateStreamingHistory(verifier *auth.KeycloakVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user, ok := authenticateUserFromRequest(verifier, w, r)
		if !ok {
			return
		}

		var req historyStreamingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		id, err := database.CreateUserStreamingSession(user.ID, database.UserStreamingSessionInput{
			SessionID:            req.SessionID,
			SourceLang:           req.SourceLang,
			TargetLang:           req.TargetLang,
			TotalChunks:          req.TotalChunks,
			TotalDurationSeconds: req.TotalDurationSeconds,
			FinalTranscript:      req.FinalTranscript,
			FinalTranslation:     req.FinalTranslation,
		})
		if err != nil {
			log.Printf("Create streaming history failed: %v", err)
			http.Error(w, "Failed to store history", http.StatusInternalServerError)
			return
		}

		writeJSON(w, historyResponse{Success: true, ID: id})
	}
}

func handleCreateUserFile(verifier *auth.KeycloakVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		user, ok := authenticateUserFromRequest(verifier, w, r)
		if !ok {
			return
		}

		var req userFileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		id, err := database.CreateUserFile(&user.ID, database.UserFileInput{
			SessionType:   req.SessionType,
			SessionID:     req.SessionID,
			BucketName:    req.BucketName,
			FileKey:       req.FileKey,
			ContentHash:   req.ContentHash,
			Etag:          req.Etag,
			MimeType:      req.MimeType,
			FileSizeBytes: req.FileSizeBytes,
			AccessedAt:    req.AccessedAt,
		})
		if err != nil {
			log.Printf("Create user file failed: %v", err)
			http.Error(w, "Failed to store file metadata", http.StatusInternalServerError)
			return
		}

		writeJSON(w, historyResponse{Success: true, ID: id})
	}
}

func extractBearerToken(r *http.Request) (string, error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return "", fmt.Errorf("Authorization header missing")
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", fmt.Errorf("Authorization header must be Bearer token")
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		return "", fmt.Errorf("Bearer token is empty")
	}

	return token, nil
}

func authenticateUserFromRequest(verifier *auth.KeycloakVerifier, w http.ResponseWriter, r *http.Request) (*database.User, bool) {
	if verifier == nil {
		http.Error(w, "Keycloak auth not configured", http.StatusServiceUnavailable)
		return nil, false
	}

	tokenStr, err := extractBearerToken(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return nil, false
	}

	claims, err := verifier.VerifyToken(r.Context(), tokenStr)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return nil, false
	}

	user, err := upsertUserFromClaims(claims)
	if err != nil {
		log.Printf("Keycloak upsert failed: %v", err)
		http.Error(w, "Failed to persist user", http.StatusInternalServerError)
		return nil, false
	}

	return user, true
}

func maybeAuthenticateUserFromRequest(verifier *auth.KeycloakVerifier, r *http.Request) (*database.User, error) {
	if verifier == nil {
		return nil, nil
	}

	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		return nil, nil
	}

	tokenStr, err := extractBearerToken(r)
	if err != nil {
		return nil, err
	}

	claims, err := verifier.VerifyToken(r.Context(), tokenStr)
	if err != nil {
		return nil, err
	}

	return upsertUserFromClaims(claims)
}

func upsertUserFromClaims(claims map[string]interface{}) (*database.User, error) {
	sub, _ := claims["sub"].(string)
	preferredUsername, _ := claims["preferred_username"].(string)
	displayName, _ := claims["name"].(string)
	email, _ := claims["email"].(string)
	emailVerified := parseEmailVerified(claims["email_verified"])

	return database.UpsertKeycloakUser(sub, preferredUsername, email, emailVerified, displayName)
}

func parseEmailVerified(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true")
	default:
		return false
	}
}

func writeJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func storageDetectContentType(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return "application/octet-stream"
	}
	if contentType := mime.TypeByExtension(ext); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func computeFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func handleVideoUpload(w http.ResponseWriter, r *http.Request, processor *video.Processor, asrClient *asr.Client, translator translate.Translator, ttsClient *tts.Client, progressMgr *progress.Manager, minioClient *storage.MinioClient, verifier *auth.KeycloakVerifier) {
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
	forceProcessing := r.FormValue("force") == "true"

	user, err := maybeAuthenticateUserFromRequest(verifier, r)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	var userID *int
	if user != nil {
		userID = &user.ID
	}

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

		var contentHash string
		if userID != nil {
			hashValue, err := computeFileHash(tempVideoPath)
			if err != nil {
				log.Printf("Failed to hash video: %v", err)
			} else {
				contentHash = hashValue
			}
		}

		if userID != nil && contentHash != "" && !forceProcessing {
			match, err := database.FindUserFileByHash(*userID, "video", contentHash)
			if err != nil {
				log.Printf("Failed to lookup video hash: %v", err)
			} else if match != nil {
				results := map[string]interface{}{
					"existing":          true,
					"existingSessionId": match.SessionID,
					"existingFileKey":   match.FileKey,
				}
				if sessionData, err := database.GetUserVideoSessionBySessionID(*userID, match.SessionID); err != nil {
					log.Printf("Failed to load existing video session: %v", err)
				} else if sessionData != nil {
					results["transcription"] = sessionData.Transcription
					results["translation"] = sessionData.Translation
					results["duration"] = float64(sessionData.DurationSeconds)
					results["minioVideoKey"] = sessionData.VideoPath
					results["minioAudioKey"] = sessionData.AudioPath
					results["minioTtsKey"] = sessionData.TTSPath
				}

				tracker.CompleteWithResults("Existing upload found", results)
				return
			}
		}

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
		translation, err := translateWithChunking(translator, transcription, sourceLang, targetLang)
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

		var minioOriginalKey string
		var minioAudioKey string
		var minioTTSKey string

		if minioClient != nil && minioClient.Enabled() {
			ctx := context.Background()

			originalKey := storage.SafeObjectKey("videos", sessionID, fmt.Sprintf("original_%s", header.Filename))
			etag, size, err := minioClient.UploadFile(ctx, originalKey, tempVideoPath, "")
			if err != nil {
				log.Printf("MinIO upload failed (original video): %v", err)
			} else {
				minioOriginalKey = originalKey
				if userID != nil {
					_, _ = database.CreateUserFile(userID, database.UserFileInput{
						SessionType:   "video",
						SessionID:     sessionID,
						BucketName:    minioClient.Bucket(),
						FileKey:       originalKey,
						ContentHash:   contentHash,
						Etag:          etag,
						MimeType:      storageDetectContentType(header.Filename),
						FileSizeBytes: size,
					})
				}
			}

			audioKey := storage.SafeObjectKey("videos", sessionID, "extracted_audio.wav")
			etag, size, err = minioClient.UploadBytes(ctx, audioKey, audioResult.AudioData, "audio/wav")
			if err != nil {
				log.Printf("MinIO upload failed (extracted audio): %v", err)
			} else {
				minioAudioKey = audioKey
				if userID != nil {
					_, _ = database.CreateUserFile(userID, database.UserFileInput{
						SessionType:   "video",
						SessionID:     sessionID,
						BucketName:    minioClient.Bucket(),
						FileKey:       audioKey,
						Etag:          etag,
						MimeType:      "audio/wav",
						FileSizeBytes: size,
					})
				}
			}

			if generateTTS && videoPath != "" {
				translatedKey := storage.SafeObjectKey("videos", sessionID, fmt.Sprintf("translated_%s", filepath.Base(videoPath)))
				etag, size, err = minioClient.UploadFile(ctx, translatedKey, filepath.Join(tempDir, videoPath), "")
				if err != nil {
					log.Printf("MinIO upload failed (translated video): %v", err)
				} else {
					minioTTSKey = translatedKey
					if userID != nil {
						_, _ = database.CreateUserFile(userID, database.UserFileInput{
							SessionType:   "video",
							SessionID:     sessionID,
							BucketName:    minioClient.Bucket(),
							FileKey:       translatedKey,
							Etag:          etag,
							MimeType:      storageDetectContentType(videoPath),
							FileSizeBytes: size,
						})
					}
				}
			}
		}

		// Send completion with results
		results := map[string]interface{}{
			"transcription": transcription,
			"translation":   translation,
			"duration":      audioResult.Duration,
			"videoPath":     videoPath,
			"minioBucket":   "",
			"minioVideoKey": minioOriginalKey,
			"minioAudioKey": minioAudioKey,
			"minioTtsKey":   minioTTSKey,
		}
		if minioClient != nil && minioClient.Enabled() {
			results["minioBucket"] = minioClient.Bucket()
		}
		if detectedLang != "" {
			results["detectedLang"] = detectedLang
		}
		tracker.CompleteWithResults("Video processing completed successfully", results)
		log.Printf("Video processing completed for session %s", sessionID)
	}() // End of goroutine
}

func handleAudioUpload(w http.ResponseWriter, r *http.Request, processor *video.Processor, asrClient *asr.Client, translator translate.Translator, progressMgr *progress.Manager, minioClient *storage.MinioClient, verifier *auth.KeycloakVerifier) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form first (max 100MB)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		log.Printf("Error parsing form: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "Failed to parse upload",
		})
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		log.Printf("Error getting file: %v", err)
		json.NewEncoder(w).Encode(videoUploadResponse{
			Success: false,
			Error:   "No audio file provided",
		})
		return
	}

	// Generate session ID for progress tracking
	sessionID := fmt.Sprintf("audio_%d", time.Now().UnixNano())

	// Read form values before starting goroutine
	targetLang := r.FormValue("targetLang")
	if targetLang == "" {
		targetLang = "en" // Default to English
	}

	sourceLang := r.FormValue("sourceLang")
	if sourceLang == "" {
		sourceLang = "auto" // Default to auto-detect
	}
	autoDetect := sourceLang == "auto" || sourceLang == "detect"

	// Check if user wants speaker diarization
	enableDiarization := r.FormValue("enableDiarization") == "true"
	enhanceAudio := r.FormValue("enhanceAudio") == "true"
	forceProcessing := r.FormValue("force") == "true"

	user, err := maybeAuthenticateUserFromRequest(verifier, r)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	var userID *int
	if user != nil {
		userID = &user.ID
	}

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

		log.Printf("Processing audio: %s (%.2f MB), source: %s, target: %s", header.Filename, float64(header.Size)/(1024*1024), sourceLang, targetLang)

		tracker.Update("saving", 20, "Saving audio file...")

		// Save uploaded file temporarily
		tempDir := processor.TempDir
		tempAudioPath := filepath.Join(tempDir, fmt.Sprintf("upload_audio_%d_%s", time.Now().Unix(), header.Filename))
		defer os.Remove(tempAudioPath)

		outFile, err := os.Create(tempAudioPath)
		if err != nil {
			log.Printf("Error creating temp file: %v", err)
			tracker.Error("saving", "Failed to save audio", err)
			return
		}

		if _, err := io.Copy(outFile, file); err != nil {
			outFile.Close()
			log.Printf("Error copying file: %v", err)
			tracker.Error("saving", "Failed to save audio", err)
			return
		}
		outFile.Close()

		var contentHash string
		if userID != nil {
			hashValue, err := computeFileHash(tempAudioPath)
			if err != nil {
				log.Printf("Failed to hash audio: %v", err)
			} else {
				contentHash = hashValue
			}
		}

		if userID != nil && contentHash != "" && !forceProcessing {
			match, err := database.FindUserFileByHash(*userID, "audio", contentHash)
			if err != nil {
				log.Printf("Failed to lookup audio hash: %v", err)
			} else if match != nil {
				results := map[string]interface{}{
					"existing":          true,
					"existingSessionId": match.SessionID,
					"existingFileKey":   match.FileKey,
				}
				if sessionData, err := database.GetUserAudioSessionBySessionID(*userID, match.SessionID); err != nil {
					log.Printf("Failed to load existing audio session: %v", err)
				} else if sessionData != nil {
					results["transcription"] = sessionData.Transcription
					results["translation"] = sessionData.Translation
					results["minioAudioKey"] = sessionData.AudioPath
					results["num_speakers"] = sessionData.NumSpeakers
					results["segments"] = sessionData.Segments
				}

				tracker.CompleteWithResults("Existing upload found", results)
				return
			}
		}

		if enhanceAudio {
			tracker.Update("processing", 30, "Cleaning up audio and converting to WAV...")
		} else {
			tracker.Update("processing", 30, "Converting audio to WAV format...")
		}

		// Convert audio to WAV format
		log.Println("Converting audio to WAV...")
		audioResult, err := processor.ConvertAudioToWAVWithEnhancement(tempAudioPath, enhanceAudio)
		if err != nil && enhanceAudio {
			log.Printf("Audio enhancement failed, retrying without enhancement: %v", err)
			audioResult, err = processor.ConvertAudioToWAVWithEnhancement(tempAudioPath, false)
		}
		if err != nil {
			log.Printf("Error converting audio: %v", err)
			tracker.Error("processing", "Failed to convert audio", err)
			return
		}

		log.Printf("Audio converted: %.2f seconds, %d bytes", audioResult.Duration, len(audioResult.AudioData))
		tracker.Update("processing", 40, fmt.Sprintf("Audio converted: %.2f seconds", audioResult.Duration))

		// Auto-detect language if requested
		var detectedLang string
		if autoDetect {
			tracker.Update("detection", 45, "Detecting language...")
			log.Println("Auto-detecting language...")
			detectedLang, err = asrClient.DetectLanguage(audioResult.AudioData)
			if err != nil {
				log.Printf("Error detecting language: %v, defaulting to 'en'", err)
				detectedLang = "en"
				sourceLang = "en" // Update sourceLang for transcription
				tracker.Update("detection", 50, "Language detection failed, using English")
			} else {
				log.Printf("Detected language: %s", detectedLang)
				sourceLang = detectedLang
				tracker.Update("detection", 50, fmt.Sprintf("Detected language: %s", detectedLang))
			}
		}

		// Transcribe audio (with or without diarization)
		tracker.Update("transcription", 60, "Transcribing audio...")
		log.Println("Transcribing audio...")

		var transcription string
		var segments []map[string]interface{}
		var numSpeakers int

		if enableDiarization {
			// Use diarization endpoint
			tracker.Update("transcription", 60, "Transcribing with speaker identification...")
			log.Println("Using speaker diarization...")

			diarizationResult, err := asrClient.TranscribeWithDiarization(audioResult.AudioData, sourceLang)
			if err != nil {
				log.Printf("Error with diarization, falling back to normal transcription: %v", err)
				// Fallback to normal transcription
				transcription, err = asrClient.TranscribeWAV(audioResult.AudioData, sourceLang)
				if err != nil {
					log.Printf("Error transcribing: %v", err)
					tracker.Error("transcription", "Failed to transcribe audio", err)
					return
				}
			} else {
				transcription = diarizationResult.Text
				segments = diarizationResult.Segments
				numSpeakers = diarizationResult.NumSpeakers
				log.Printf("Diarization complete: %d speakers, %d segments", numSpeakers, len(segments))
			}
		} else {
			// Normal transcription
			transcription, err = asrClient.TranscribeWAV(audioResult.AudioData, sourceLang)
			if err != nil {
				log.Printf("Error transcribing: %v", err)
				tracker.Error("transcription", "Failed to transcribe audio", err)
				return
			}
		}

		log.Printf("Transcription: %s", transcription[:min(len(transcription), 100)])
		tracker.Update("transcription", 75, "Transcription complete")

		// Translate transcription
		var translation string

		if len(segments) > 0 {
			// Translate each segment
			tracker.Update("translation", 80, fmt.Sprintf("Translating %d segments...", len(segments)))
			log.Printf("Translating %d segments from %s to %s...", len(segments), sourceLang, targetLang)

			for i, seg := range segments {
				segText := seg["text"].(string)
				translatedText, err := translateWithChunking(translator, segText, sourceLang, targetLang)
				if err != nil {
					log.Printf("Error translating segment %d: %v", i, err)
					translatedText = segText // Fallback to original
				}
				seg["translation"] = translatedText
				segments[i] = seg
			}

			// Also create full translation
			translation, _ = translateWithChunking(translator, transcription, sourceLang, targetLang)
		} else {
			// Single translation
			tracker.Update("translation", 80, fmt.Sprintf("Translating from %s to %s...", sourceLang, targetLang))
			log.Printf("Translating from %s to %s...", sourceLang, targetLang)
			translation, err = translateWithChunking(translator, transcription, sourceLang, targetLang)
			if err != nil {
				log.Printf("Error translating: %v", err)
				tracker.Error("translation", "Failed to translate", err)
				return
			}
		}

		log.Printf("Translation complete")
		tracker.Update("translation", 90, "Translation complete")

		var minioAudioKey string
		if minioClient != nil && minioClient.Enabled() {
			ctx := context.Background()
			audioKey := storage.SafeObjectKey("audio", sessionID, fmt.Sprintf("original_%s", header.Filename))
			etag, size, err := minioClient.UploadFile(ctx, audioKey, tempAudioPath, "")
			if err != nil {
				log.Printf("MinIO upload failed (audio): %v", err)
			} else {
				minioAudioKey = audioKey
				if userID != nil {
					_, _ = database.CreateUserFile(userID, database.UserFileInput{
						SessionType:   "audio",
						SessionID:     sessionID,
						BucketName:    minioClient.Bucket(),
						FileKey:       audioKey,
						ContentHash:   contentHash,
						Etag:          etag,
						MimeType:      storageDetectContentType(header.Filename),
						FileSizeBytes: size,
					})
				}
			}
		}

		// Send completion with results
		results := map[string]interface{}{
			"transcription": transcription,
			"translation":   translation,
			"minioBucket":   "",
			"minioAudioKey": minioAudioKey,
		}
		if minioClient != nil && minioClient.Enabled() {
			results["minioBucket"] = minioClient.Bucket()
		}
		if detectedLang != "" {
			results["detectedLang"] = detectedLang
		}
		if len(segments) > 0 {
			results["segments"] = segments
			results["num_speakers"] = numSpeakers
		}
		tracker.CompleteWithResults("Audio processing completed successfully", results)
		log.Printf("Audio processing completed for session %s", sessionID)
	}() // End of goroutine
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Meeting API Handlers

func handleCreateMeeting(w http.ResponseWriter, r *http.Request, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		Mode string `json:"mode"` // "individual" or "shared"
	}

	// Try to parse JSON, but don't fail if empty (default to individual)
	json.NewDecoder(r.Body).Decode(&req)

	// Default to individual mode
	if req.Mode == "" {
		req.Mode = "individual"
	}

	// Validate mode
	if req.Mode != "individual" && req.Mode != "shared" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid mode. Must be 'individual' or 'shared'",
		})
		return
	}

	user, err := maybeAuthenticateUserFromRequest(keycloakVerifier, r)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	var userID *int
	if user != nil {
		userID = &user.ID
	}

	// Create meeting in database
	meeting, err := database.CreateMeeting(userID, req.Mode)
	if err != nil {
		log.Printf("Error creating meeting: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to create meeting",
		})
		return
	}

	log.Printf("Created meeting: %s (room code: %s, mode: %s)", meeting.ID, meeting.RoomCode, meeting.Mode)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"meetingId": meeting.ID,
		"roomCode":  meeting.RoomCode,
		"mode":      meeting.Mode,
		"hostToken": meeting.HostToken,
	})
}

func handleJoinMeeting(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract room code from URL path
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid room code", http.StatusBadRequest)
		return
	}
	roomCode := pathParts[3]

	// Parse request body
	var req struct {
		ParticipantName string `json:"participantName"`
		TargetLanguage  string `json:"targetLanguage"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.ParticipantName == "" {
		http.Error(w, "Participant name is required", http.StatusBadRequest)
		return
	}
	if req.TargetLanguage == "" {
		req.TargetLanguage = "en" // Default to English
	}

	// Get meeting by room code
	mtg, err := database.GetMeetingByRoomCode(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to find meeting",
		})
		return
	}

	if mtg == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Meeting not found",
		})
		return
	}

	if !mtg.IsActive {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Meeting has ended",
		})
		return
	}

	user, err := maybeAuthenticateUserFromRequest(keycloakVerifier, r)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}
	var userID *int
	if user != nil {
		userID = &user.ID
	}

	// Add participant to database
	participant, err := database.AddParticipant(mtg.ID, userID, req.ParticipantName, req.TargetLanguage)
	if err != nil {
		log.Printf("Error adding participant: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to join meeting",
		})
		return
	}

	log.Printf("Participant %d (%s) joined meeting %s", participant.ID, participant.ParticipantName, mtg.ID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"participantId": participant.ID,
		"meetingId":     mtg.ID,
	})
}

func handleGetMeeting(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract room code from URL path: /api/meetings/K1N-G-A
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 {
		http.Error(w, "Invalid room code", http.StatusBadRequest)
		return
	}
	roomCode := pathParts[3]

	// Get meeting by room code or ID
	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to find meeting",
		})
		return
	}

	if mtg == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Meeting not found",
		})
		return
	}

	// Get active participants from database
	participants, err := database.GetActiveParticipants(mtg.ID)
	if err != nil {
		log.Printf("Error getting participants: %v", err)
		participants = []database.MeetingParticipant{} // Return empty array on error
	}

	// Convert to response format
	participantList := make([]map[string]interface{}, len(participants))
	for i, p := range participants {
		participantList[i] = map[string]interface{}{
			"id":             p.ID,
			"name":           p.ParticipantName,
			"targetLanguage": p.TargetLanguage,
			"joinedAt":       p.JoinedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":      true,
		"meetingId":    mtg.ID,
		"roomCode":     mtg.RoomCode,
		"mode":         mtg.Mode,
		"isActive":     mtg.IsActive,
		"participants": participantList,
	})
}

func handleUpdateSpeakerName(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager, roomCode, speakerID string) {
	// Parse request body
	var req struct {
		SpeakerName string `json:"speakerName"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate inputs
	if req.SpeakerName == "" {
		http.Error(w, "Speaker name is required", http.StatusBadRequest)
		return
	}

	// Get meeting by room code or ID
	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to find meeting",
		})
		return
	}

	if mtg == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Meeting not found",
		})
		return
	}

	if !mtg.IsActive {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Meeting has ended",
		})
		return
	}

	// Save speaker name mapping to database
	if err := database.SetSpeakerName(mtg.ID, speakerID, req.SpeakerName); err != nil {
		log.Printf("Error saving speaker name: %v", err)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Failed to save speaker name",
		})
		return
	}

	log.Printf("Updated speaker %s in meeting %s to name: %s", speakerID, mtg.ID, req.SpeakerName)

	// Broadcast update to all participants in the room
	roomManager.Broadcast(mtg.ID, meeting.Message{
		Type:        "speaker_name_updated",
		SpeakerID:   speakerID,
		SpeakerName: req.SpeakerName,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"speakerId":   speakerID,
		"speakerName": req.SpeakerName,
	})
}

func handleDownloadTranscript(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager, roomCode string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		http.Error(w, "lang is required", http.StatusBadRequest)
		return
	}

	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		http.Error(w, "Failed to find meeting", http.StatusNotFound)
		return
	}
	if mtg == nil {
		http.Error(w, "Meeting not found", http.StatusNotFound)
		return
	}

	entries := roomManager.GetTranscript(mtg.ID, lang)
	content := formatTranscript(entries)

	filename := fmt.Sprintf("meeting_%s_%s.txt", mtg.RoomCode, lang)
	if mtg.RoomCode == "" {
		filename = fmt.Sprintf("meeting_%s_%s.txt", mtg.ID, lang)
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(content)); err != nil {
		log.Printf("Failed to write transcript response: %v", err)
	}
}

func handleDownloadTranscriptSnapshot(w http.ResponseWriter, r *http.Request, roomCode string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lang := r.URL.Query().Get("lang")
	if lang == "" {
		http.Error(w, "lang is required", http.StatusBadRequest)
		return
	}

	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		http.Error(w, "Failed to find meeting", http.StatusNotFound)
		return
	}
	if mtg == nil {
		http.Error(w, "Meeting not found", http.StatusNotFound)
		return
	}

	snapshot, err := database.GetMeetingTranscriptSnapshot(mtg.ID, lang)
	if err != nil {
		log.Printf("Failed to get transcript snapshot: %v", err)
		http.Error(w, "Failed to load transcript snapshot", http.StatusInternalServerError)
		return
	}
	if snapshot == nil || snapshot.Transcript == "" {
		http.Error(w, "Transcript snapshot not found", http.StatusNotFound)
		return
	}

	filename := fmt.Sprintf("meeting_%s_%s_snapshot.txt", mtg.RoomCode, lang)
	if mtg.RoomCode == "" {
		filename = fmt.Sprintf("meeting_%s_%s_snapshot.txt", mtg.ID, lang)
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(snapshot.Transcript)); err != nil {
		log.Printf("Failed to write transcript snapshot response: %v", err)
	}
}

func handleListTranscriptSnapshots(w http.ResponseWriter, r *http.Request, roomCode string) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		http.Error(w, "Failed to find meeting", http.StatusNotFound)
		return
	}
	if mtg == nil {
		http.Error(w, "Meeting not found", http.StatusNotFound)
		return
	}

	snapshots, err := database.ListMeetingTranscriptSnapshots(mtg.ID)
	if err != nil {
		log.Printf("Failed to list transcript snapshots: %v", err)
		http.Error(w, "Failed to list transcript snapshots", http.StatusInternalServerError)
		return
	}

	type snapshotInfo struct {
		Language  string `json:"language"`
		CreatedAt string `json:"createdAt"`
	}

	items := make([]snapshotInfo, 0, len(snapshots))
	for _, snapshot := range snapshots {
		items = append(items, snapshotInfo{
			Language:  snapshot.Language,
			CreatedAt: snapshot.CreatedAt.Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"snapshots": items,
	})
}

func handleEndMeeting(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager, llmClient *llm.Client, roomCode string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		HostToken string `json:"hostToken"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.HostToken == "" {
		http.Error(w, "Host token required", http.StatusBadRequest)
		return
	}

	mtg, err := getMeetingByCodeOrID(roomCode)
	if err != nil {
		log.Printf("Error getting meeting: %v", err)
		http.Error(w, "Failed to find meeting", http.StatusNotFound)
		return
	}
	if mtg == nil {
		http.Error(w, "Meeting not found", http.StatusNotFound)
		return
	}

	valid, err := database.ValidateMeetingHostToken(mtg.ID, req.HostToken)
	if err != nil {
		log.Printf("Failed to validate host token: %v", err)
		http.Error(w, "Failed to validate host token", http.StatusInternalServerError)
		return
	}
	if !valid {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := roomManager.EndMeeting(mtg.ID); err != nil {
		log.Printf("Failed to end meeting: %v", err)
		http.Error(w, "Failed to end meeting", http.StatusInternalServerError)
		return
	}

	if llmClient != nil {
		go func() {
			if err := meeting.GenerateMeetingMinutes(mtg.ID, "en", llmClient); err != nil {
				log.Printf("Minutes generation failed for meeting %s: %v", mtg.ID, err)
			}
		}()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func formatTranscript(entries []meeting.TranscriptEntry) string {
	if len(entries) == 0 {
		return ""
	}
	var b strings.Builder
	for _, entry := range entries {
		speaker := entry.SpeakerName
		if speaker == "" {
			speaker = entry.SpeakerID
		}
		if speaker == "" {
			speaker = "Speaker"
		}
		ts := entry.Timestamp.Format("15:04:05")
		b.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, speaker, entry.Text))
	}
	return b.String()
}


func getMeetingByCodeOrID(codeOrID string) (*database.Meeting, error) {
	mtg, err := database.GetMeetingByRoomCode(codeOrID)
	if err != nil {
		return nil, err
	}
	if mtg != nil {
		return mtg, nil
	}
	return database.GetMeetingByID(codeOrID)
}

func handleMeetingOperations(w http.ResponseWriter, r *http.Request, roomManager *meeting.RoomManager, llmClient *llm.Client, keycloakVerifier *auth.KeycloakVerifier) {
	// Route based on URL pattern
	// /api/meetings/{roomCode} - GET meeting info
	// /api/meetings/{roomCode}/join - POST to join
	// /api/meetings/{roomCode}/speakers/{speakerId} - POST to update speaker name
	// /api/meetings/{roomCode}/transcript - GET to download transcript (lang query param)
	// /api/meetings/{roomCode}/transcript-snapshots - GET to list available snapshots
	// /api/meetings/{roomCode}/transcript-snapshot - GET to download snapshot (lang query param)
	// /api/meetings/{roomCode}/end - POST to end meeting (host only)
	pathParts := strings.Split(r.URL.Path, "/")

	if len(pathParts) < 4 {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}

	// Check if it's a join request
	if len(pathParts) >= 5 && pathParts[4] == "join" {
		handleJoinMeeting(w, r, roomManager, keycloakVerifier)
		return
	}

	// Check if it's a transcript download: /api/meetings/{roomCode}/transcript
	if len(pathParts) >= 5 && pathParts[4] == "transcript" && r.Method == "GET" {
		handleDownloadTranscript(w, r, roomManager, pathParts[3])
		return
	}

	// Check if it's a transcript snapshot download
	if len(pathParts) >= 5 && pathParts[4] == "transcript-snapshot" && r.Method == "GET" {
		handleDownloadTranscriptSnapshot(w, r, pathParts[3])
		return
	}

	// Check if it's a transcript snapshot list
	if len(pathParts) >= 5 && pathParts[4] == "transcript-snapshots" && r.Method == "GET" {
		handleListTranscriptSnapshots(w, r, pathParts[3])
		return
	}

	// Check if it's an end meeting request
	if len(pathParts) >= 5 && pathParts[4] == "end" && r.Method == "POST" {
		handleEndMeeting(w, r, roomManager, llmClient, pathParts[3])
		return
	}

	// Check if it's a speaker name update: /api/meetings/{roomCode}/speakers/{speakerId}
	if len(pathParts) >= 6 && pathParts[4] == "speakers" && r.Method == "POST" {
		handleUpdateSpeakerName(w, r, roomManager, pathParts[3], pathParts[5])
		return
	}

	// Otherwise, it's a get meeting info request
	handleGetMeeting(w, r, roomManager)
}

func handleSpeakerProfiles(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/api/speaker-profiles/")
	if sessionID == "" || sessionID == r.URL.Path {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		profiles, err := database.GetSpeakerProfiles(sessionID)
		if err != nil {
			log.Printf("Failed to get speaker profiles: %v", err)
			http.Error(w, "Failed to get speaker profiles", http.StatusInternalServerError)
			return
		}

		type profileInfo struct {
			ProfileID string    `json:"profileId"`
			Embedding []float32 `json:"embedding"`
			Count     int       `json:"count"`
			UpdatedAt string    `json:"updatedAt"`
		}

		response := make([]profileInfo, 0, len(profiles))
		for _, profile := range profiles {
			response = append(response, profileInfo{
				ProfileID: profile.ProfileID,
				Embedding: profile.Embedding,
				Count:     profile.Count,
				UpdatedAt: profile.UpdatedAt.Format(time.RFC3339),
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"profiles": response,
		})
	case http.MethodPut:
		type profilePayload struct {
			ProfileID string    `json:"profileId"`
			Embedding []float32 `json:"embedding"`
			Count     int       `json:"count"`
		}
		var payload struct {
			Profiles []profilePayload `json:"profiles"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid payload", http.StatusBadRequest)
			return
		}

		profiles := make([]database.SpeakerProfile, 0, len(payload.Profiles))
		for _, item := range payload.Profiles {
			if item.ProfileID == "" || len(item.Embedding) == 0 {
				continue
			}
			count := item.Count
			if count <= 0 {
				count = 1
			}
			profiles = append(profiles, database.SpeakerProfile{
				SessionID: sessionID,
				ProfileID: item.ProfileID,
				Embedding: item.Embedding,
				Count:     count,
			})
		}

		if err := database.ReplaceSpeakerProfiles(sessionID, profiles); err != nil {
			log.Printf("Failed to persist speaker profiles: %v", err)
			http.Error(w, "Failed to persist speaker profiles", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  true,
			"profiles": len(profiles),
		})
	case http.MethodDelete:
		if err := database.DeleteSpeakerProfiles(sessionID); err != nil {
			log.Printf("Failed to delete speaker profiles: %v", err)
			http.Error(w, "Failed to delete speaker profiles", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleSpeakerProfileCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ttlSeconds := int64(0)
	if ttlParam := r.URL.Query().Get("ttl_seconds"); ttlParam != "" {
		if parsed, err := strconv.ParseInt(ttlParam, 10, 64); err == nil {
			ttlSeconds = parsed
		}
	}

	if ttlSeconds <= 0 {
		ttlEnv := os.Getenv("SPEAKER_PROFILE_DB_TTL_SECONDS")
		if ttlEnv != "" {
			if parsed, err := strconv.ParseInt(ttlEnv, 10, 64); err == nil {
				ttlSeconds = parsed
			}
		}
	}

	if ttlSeconds <= 0 {
		http.Error(w, "ttl_seconds is required", http.StatusBadRequest)
		return
	}

	cutoff := time.Now().Add(-time.Duration(ttlSeconds) * time.Second)
	deleted, err := database.DeleteExpiredSpeakerProfiles(cutoff)
	if err != nil {
		log.Printf("Failed to delete expired speaker profiles: %v", err)
		http.Error(w, "Failed to delete expired speaker profiles", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"deletedRows": deleted,
	})
}

func main() {
	// Initialize database
	log.Println("Initializing database connection...")
	if err := database.Init(); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()
	log.Println("Database connection established")

	// Create RAG processor (will be initialized after embedding client is created)
	var roomManager *meeting.RoomManager

	log.Println("Meeting room manager will be initialized after RAG components")

	// Optional speaker profile cleanup job
	if ttlEnv := os.Getenv("SPEAKER_PROFILE_DB_TTL_SECONDS"); ttlEnv != "" {
		if ttlSeconds, err := strconv.ParseInt(ttlEnv, 10, 64); err == nil && ttlSeconds > 0 {
			intervalSeconds := int64(300)
			if intervalEnv := os.Getenv("SPEAKER_PROFILE_DB_CLEANUP_INTERVAL_SECONDS"); intervalEnv != "" {
				if parsed, err := strconv.ParseInt(intervalEnv, 10, 64); err == nil && parsed > 0 {
					intervalSeconds = parsed
				}
			}

			go func() {
				ticker := time.NewTicker(time.Duration(intervalSeconds) * time.Second)
				defer ticker.Stop()
				for range ticker.C {
					cutoff := time.Now().Add(-time.Duration(ttlSeconds) * time.Second)
					deleted, err := database.DeleteExpiredSpeakerProfiles(cutoff)
					if err != nil {
						log.Printf("Speaker profile cleanup failed: %v", err)
						continue
					}
					if deleted > 0 {
						log.Printf("Speaker profile cleanup removed %d row(s)", deleted)
					}
				}
			}()
		}
	}

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

	// Create RAG components (embedding + LLM clients)
	embeddingClient := embedding.New("http://127.0.0.1:8006")
	llmClient := llm.New("http://127.0.0.1:8007")
	ragProcessor := rag.NewProcessor(embeddingClient)
	ragQueryEngine := rag.NewQueryEngine(embeddingClient, llmClient)
	log.Println("RAG components initialized")

	// Initialize RoomManager with RAG processor
	roomManager = meeting.NewRoomManager(ragProcessor)
	log.Println("Meeting room manager initialized with RAG support")

	keycloakVerifier, err := auth.NewKeycloakVerifierFromEnv()
	if err != nil {
		log.Printf("Keycloak auth disabled: %v", err)
	}

	minioClient, err := storage.NewMinioFromEnv()
	if err != nil {
		log.Printf("MinIO disabled: %v", err)
	}

	// Static file server
	http.Handle("/", http.FileServer(http.Dir("./web")))

	// Redirects for new file structure
	http.HandleFunc("/meeting.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/meeting/meeting-create.html")
	})
	http.HandleFunc("/meeting-join.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/meeting/meeting-join.html")
	})
	http.HandleFunc("/meeting-room.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/meeting/meeting-room.html")
	})
	http.HandleFunc("/streaming.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/streaming/streaming.html")
	})
	http.HandleFunc("/recording.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/recording/recording.html")
	})
	http.HandleFunc("/video.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/features/video/video.html")
	})

	// JavaScript file redirects for compatibility
	http.HandleFunc("/meeting-room.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "./web/features/meeting/meeting-room.js")
	})
	http.HandleFunc("/assets/js/audio-processor.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "./web/assets/js/audio-processor.js")
	})
	http.HandleFunc("/assets/js/utils.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, r, "./web/assets/js/utils.js")
	})

	http.HandleFunc("/api/speaker-profiles/cleanup", handleSpeakerProfileCleanup)
	http.HandleFunc("/api/speaker-profiles/", handleSpeakerProfiles)
	http.HandleFunc("/api/auth/keycloak", handleKeycloakLogin(keycloakVerifier))
	http.HandleFunc("/api/history/video", handleCreateVideoHistory(keycloakVerifier))
	http.HandleFunc("/api/history/audio", handleCreateAudioHistory(keycloakVerifier))
	http.HandleFunc("/api/history/streaming", handleCreateStreamingHistory(keycloakVerifier))
	http.HandleFunc("/api/files", handleCreateUserFile(keycloakVerifier))

	// User meetings history API endpoints
	http.HandleFunc("/api/users/me/meetings", func(w http.ResponseWriter, r *http.Request) {
		handleListUserMeetings(w, r, keycloakVerifier)
	})
	http.HandleFunc("/api/users/me/meetings/", func(w http.ResponseWriter, r *http.Request) {
		handleGetUserMeetingDetail(w, r, keycloakVerifier)
	})

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("upgrade:", err)
			return
		}
		go srv.HandleConn(conn)
	})

	http.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		handleVideoUpload(w, r, videoProcessor, asrClient, translator, ttsClient, progressMgr, minioClient, keycloakVerifier)
	})

	http.HandleFunc("/upload-audio", func(w http.ResponseWriter, r *http.Request) {
		handleAudioUpload(w, r, videoProcessor, asrClient, translator, progressMgr, minioClient, keycloakVerifier)
	})

	// Meeting API endpoints
	http.HandleFunc("/api/meetings", func(w http.ResponseWriter, r *http.Request) {
		handleCreateMeeting(w, r, keycloakVerifier)
	})
	http.HandleFunc("/api/meetings/", func(w http.ResponseWriter, r *http.Request) {
		handleMeetingOperations(w, r, roomManager, llmClient, keycloakVerifier)
	})

	// RAG Chat API endpoints
	http.HandleFunc("/api/chat/sessions", func(w http.ResponseWriter, r *http.Request) {
		handleChatSessions(w, r, keycloakVerifier)
	})
	http.HandleFunc("/api/chat/query", func(w http.ResponseWriter, r *http.Request) {
		handleChatQuery(w, r, ragQueryEngine, keycloakVerifier)
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

	// Meeting WebSocket - for real-time meeting rooms
	http.HandleFunc("/ws/meeting/", func(w http.ResponseWriter, r *http.Request) {
		// Extract meeting ID from URL path
		pathParts := strings.Split(r.URL.Path, "/")
		if len(pathParts) < 4 {
			http.Error(w, "Invalid meeting ID", http.StatusBadRequest)
			return
		}
		meetingID := pathParts[3]

		// Get query parameters
		query := r.URL.Query()
		participantIDStr := query.Get("participantId")
		participantName := query.Get("participantName")
		targetLang := query.Get("targetLang")
		minSpeakersStr := query.Get("minSpeakers")
		maxSpeakersStr := query.Get("maxSpeakers")
		strictnessStr := query.Get("strictness")

		// Validate parameters
		if participantIDStr == "" || participantName == "" || targetLang == "" {
			http.Error(w, "Missing required parameters: participantId, participantName, targetLang", http.StatusBadRequest)
			return
		}

		// Parse participant ID
		var participantID int
		if _, err := fmt.Sscanf(participantIDStr, "%d", &participantID); err != nil {
			http.Error(w, "Invalid participant ID", http.StatusBadRequest)
			return
		}

		minSpeakers := 0
		if minSpeakersStr != "" {
			if parsed, err := strconv.Atoi(minSpeakersStr); err == nil {
				minSpeakers = parsed
			}
		}

		maxSpeakers := 0
		if maxSpeakersStr != "" {
			if parsed, err := strconv.Atoi(maxSpeakersStr); err == nil {
				maxSpeakers = parsed
			}
		}

		strictness := 0.0
		if strictnessStr != "" {
			if parsed, err := strconv.ParseFloat(strictnessStr, 64); err == nil {
				strictness = parsed
			}
		}

		// Upgrade to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Meeting WebSocket upgrade error: %v", err)
			return
		}

		// Handle the connection
		go roomManager.HandleMeetingWebSocket(conn, meetingID, participantID, participantName, targetLang, minSpeakers, maxSpeakers, strictness)
	})

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

// translateWithChunking wraps the translator to handle texts larger than 5000 characters
func translateWithChunking(t translate.Translator, text, sourceLang, targetLang string) (string, error) {
	const maxChunkSize = 5000

	// Check if the translator is an HTTPTranslator with ChunkAndTranslate method
	if httpTrans, ok := t.(*translate.HTTPTranslator); ok {
		return httpTrans.ChunkAndTranslate(text, sourceLang, targetLang)
	}

	// Fallback to regular translation for other translator types
	return t.TranslateWithSource(text, sourceLang, targetLang)
}

// handleChatSessions creates a new chat session for a meeting
func handleChatSessions(w http.ResponseWriter, r *http.Request, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MeetingID string `json:"meetingId"`
		Language  string `json:"language"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.MeetingID == "" || req.Language == "" {
		http.Error(w, "Missing required fields: meetingId, language", http.StatusBadRequest)
		return
	}

	// Optionally extract user ID from auth token
	var userID *int
	if keycloakVerifier != nil {
		tokenStr := r.Header.Get("Authorization")
		if tokenStr != "" && len(tokenStr) > 7 && tokenStr[:7] == "Bearer " {
			tokenStr = tokenStr[7:]
			claims, err := keycloakVerifier.VerifyToken(r.Context(), tokenStr)
			if err == nil {
				if preferredUsername, ok := claims["preferred_username"].(string); ok && preferredUsername != "" {
					user, _ := database.GetUserByUsername(preferredUsername)
					if user != nil {
						userID = &user.ID
					}
				}
			}
		}
	}

	session, err := database.CreateChatSession(req.MeetingID, req.Language, userID)
	if err != nil {
		log.Printf("Failed to create chat session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(session)
}

// handleChatQuery performs a RAG query on a meeting transcript
func handleChatQuery(w http.ResponseWriter, r *http.Request, queryEngine *rag.QueryEngine, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"sessionId"`
		Question  string `json:"question"`
		MeetingID string `json:"meetingId"`
		Language  string `json:"language"`
		TopK      int    `json:"topK,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.SessionID == "" || req.Question == "" || req.MeetingID == "" || req.Language == "" {
		http.Error(w, "Missing required fields: sessionId, question, meetingId, language", http.StatusBadRequest)
		return
	}

	// Default to top 5 chunks
	if req.TopK == 0 {
		req.TopK = 5
	}

	// Save user question to database
	userMsg := &database.ChatMessage{
		SessionID: req.SessionID,
		Role:      "user",
		Content:   req.Question,
	}
	if err := database.SaveChatMessage(userMsg); err != nil {
		log.Printf("Failed to save user message: %v", err)
	}

	// Update session activity
	database.UpdateChatSessionActivity(req.SessionID)

	// Perform RAG query
	answer, chunkIDs, err := queryEngine.Query(req.MeetingID, req.Language, req.Question, req.TopK)
	if err != nil {
		log.Printf("RAG query failed: %v", err)
		http.Error(w, fmt.Sprintf("Query failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Save assistant response to database
	assistantMsg := &database.ChatMessage{
		SessionID:       req.SessionID,
		Role:            "assistant",
		Content:         answer,
		ContextChunkIDs: chunkIDs,
	}
	if err := database.SaveChatMessage(assistantMsg); err != nil {
		log.Printf("Failed to save assistant message: %v", err)
	}

	// Update session activity again
	database.UpdateChatSessionActivity(req.SessionID)

	response := map[string]interface{}{
		"answer":    answer,
		"chunkIds":  chunkIDs,
		"sessionId": req.SessionID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleListUserMeetings returns all meetings for the authenticated user
func handleListUserMeetings(w http.ResponseWriter, r *http.Request, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := authenticateUserFromRequest(keycloakVerifier, w, r)
	if !ok {
		return // Response already sent
	}

	// Parse query parameters
	query := r.URL.Query()
	limit := 20
	offset := 0
	status := "all"

	if l := query.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	if o := query.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if s := query.Get("status"); s == "active" || s == "ended" {
		status = s
	}

	meetings, total, err := database.GetUserMeetings(user.ID, limit, offset, status)
	if err != nil {
		log.Printf("Failed to get user meetings: %v", err)
		http.Error(w, "Failed to get meetings", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"meetings": meetings,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// handleGetUserMeetingDetail returns detailed meeting info
func handleGetUserMeetingDetail(w http.ResponseWriter, r *http.Request, keycloakVerifier *auth.KeycloakVerifier) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := authenticateUserFromRequest(keycloakVerifier, w, r)
	if !ok {
		return // Response already sent
	}

	// Extract meeting ID from URL path: /api/users/me/meetings/{meetingId}
	path := strings.TrimPrefix(r.URL.Path, "/api/users/me/meetings/")
	meetingID := strings.TrimSuffix(path, "/")

	if meetingID == "" {
		http.Error(w, "Meeting ID required", http.StatusBadRequest)
		return
	}

	detail, err := database.GetUserMeetingDetail(user.ID, meetingID)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Meeting not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to get meeting detail: %v", err)
		http.Error(w, "Failed to get meeting", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"meeting": detail,
	})
}
