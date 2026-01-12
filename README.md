# Real-time Audio Translator

A real-time speech-to-text and translation system with live streaming, multi-user meetings, video/audio processing, meeting history, and RAG chat.

## 🌟 Features

### Live Translation Modes
- **🎙️ Real-time Streaming**: Live microphone transcription with instant translation
- **📹 Meeting Rooms**: Multi-user meetings with translation per participant
  - **Individual Device Mode**: Each person uses their own microphone
  - **Shared Room Mode**: Multiple speakers on one mic with AI speaker identification
- **🎬 Video Translation**: Upload videos for transcription, translation, and TTS audio replacement
- **🎵 Audio Recording**: Upload audio files with speaker diarization support

### Translation & Languages
- Support for 10+ languages (English, Arabic, Urdu, Spanish, French, German, Chinese, Japanese, Korean, Hindi)
- Auto-detect source language
- Real-time parallel translation to multiple languages
- Voice cloning for TTS (experimental)

### Advanced Features
- **Speaker Diarization**: Automatic speaker identification and labeling
- **Real-time Collaboration**: Multiple users in shared meeting rooms
- **Progress Tracking**: WebSocket-based progress updates for long operations
- **Audio Enhancement**: Optional noise reduction for uploaded files
- **Transcript Export**: Download meeting transcripts in multiple languages
- **Meeting History**: Account-scoped history with meeting detail views
- **RAG Chat**: Ask questions about meeting transcripts
- **Meeting Minutes**: Auto-generated participants, key points, action items, decisions, and summary

## 🏗️ Architecture

### System Components
- **Frontend**: Feature-based web UI
- **Backend**: Go server with WebSocket support and REST API
- **ASR Service**: Python FastAPI + Whisper for speech recognition
- **Translation Service**: Python service using Google Translate API
- **TTS Service**: XTTS v2 for text-to-speech with voice cloning
- **Embedding + LLM**: RAG pipeline for meeting Q&A
- **Database**: PostgreSQL for meeting data and participants
- **Video Processing**: FFmpeg for audio/video manipulation

### Streaming Captions (Live)
```
Browser Mic → WebSocket (16kHz PCM) → Faster-Whisper → Partial Captions
                                        ↓
                                     VAD
                                        ↓
                                Final Segments
                                        ↓
                                  Translation
                                        ↓
                               Translated Output
```
- Partial captions appear immediately
- Final segments are emitted after silence detection
- Translation runs on finalized segments only

### Web Directory Structure
```
web/
├── index.html                    # Landing page
├── assets/                       # Shared resources
│   ├── css/                      # Modular stylesheets
│   ├── js/                       # Shared utilities
│   └── images/                   # Static images
├── components/                   # Reusable UI components
└── features/                     # Feature modules
    ├── home/                     # Home page
    ├── streaming/                # Live streaming
    ├── recording/                # Audio upload
    ├── video/                    # Video upload
    ├── meeting/                  # Meeting rooms
    └── history/                  # Meeting history + chat
```

## 📋 Prerequisites

- **Go** 1.16+ (backend server)
- **Python** 3.8+ (AI services)
- **Docker & Docker Compose** (recommended)
- **FFmpeg** (video/audio processing)
- **PostgreSQL** 15+

### Install FFmpeg
```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg
```

## 🚀 Quick Start

### Automated Setup (Recommended)
```bash
# 1. Clone the repository
git clone <your-repo>
cd audio-translator

# 2. Create .env file
cp .env.example .env
# Edit .env and add your HuggingFace token for speaker diarization

# 3. Start all services
./start-services.sh
```

**What this does:**
- ✅ Checks and builds Docker images if needed
- ✅ Starts PostgreSQL, ASR, Translation, TTS, Embedding, and LLM services
- ✅ Runs database migrations
- ✅ Builds and starts the Go web server

**Service URLs:**
- 🌐 Web UI: http://localhost:8080
- 🎤 ASR Service: http://localhost:8003
- 🌍 Translation: http://localhost:8004
- 🔊 TTS Service: http://localhost:8005
- 🧠 Embeddings: http://localhost:8006
- 💬 LLM: http://localhost:8007

### Manual Docker Setup
```bash
# Start services
docker compose up -d

# Run database migrations
cat migrations/*.sql | docker exec -i audio-translator-postgres-1 psql -U audio_translator -d audio_translator

# Start Go server
go build -o bin/server cmd/server/main.go
set -a && source .env && set +a  # Load environment variables
./bin/server
```

### Stop Services
```bash
# Stop everything
docker compose down && pkill -f bin/server

# Stop just Docker containers
docker compose down

# Stop just Go server
kill $(cat bin/server.pid)
```

## 📖 Usage Guide

### 1. Real-time Streaming
1. Go to http://localhost:8080
2. Click **"Streaming Translation"**
3. Select source and target languages
4. Click **Start** and grant microphone permission
5. Speak into your microphone
6. See real-time transcription and translation
7. Download transcript when done

### 2. Meeting Rooms
1. Go to http://localhost:8080/meeting.html
2. Choose **Individual Devices** or **Shared Room**
3. Share the room code with participants
4. Join, select language, and grant microphone permission
5. Host can end the meeting for everyone

### 3. Meeting History + RAG Chat
1. Go to http://localhost:8080/features/history/meetings-history.html
2. Sign in (Keycloak) to view account-scoped history
3. Open a meeting to view minutes and full transcript
4. Use the chat panel to ask questions about the meeting

### 4. Video Translation
1. Go to http://localhost:8080/video.html
2. Upload a video file
3. Select source/target languages
4. Optional: enable **"Generate translated audio"** and **"Clone original voice"**
5. Process and download results

### 5. Audio Recording
1. Go to http://localhost:8080/recording.html
2. Upload an audio file
3. Optional: enable **Speaker Diarization** or **Audio Enhancement**
4. Process and view results

## 🔧 Configuration

### Environment Variables (.env)
```bash
# HuggingFace token (required for diarization)
HF_TOKEN=your_huggingface_token_here

# CORS (optional - leave empty for development)
ALLOWED_ORIGINS=

# Diarization tuning
SPEAKER_SIM_THRESHOLD=0.82
MIN_EMBED_DURATION=0.8
SPEAKER_OVERLAP_RATIO_THRESHOLD=0.25
SPEAKER_CONFIDENCE_THRESHOLD=0.55
SPEAKER_PROFILE_TTL_SECONDS=3600
SPEAKER_PROFILE_CLEANUP_INTERVAL_SECONDS=300
SPEAKER_PROFILE_STORE_URL=
SPEAKER_PROFILE_PERSIST_INTERVAL_SECONDS=15
SPEAKER_PROFILE_DB_TTL_SECONDS=86400
SPEAKER_PROFILE_DB_CLEANUP_INTERVAL_SECONDS=300

# Keycloak JWT verification
KEYCLOAK_ISSUER=
KEYCLOAK_JWKS_URL=
KEYCLOAK_AUDIENCE=

# Backend service URLs
ASR_BASE_URL=http://127.0.0.1:8003
TRANSLATION_BASE_URL=http://127.0.0.1:8004
TTS_BASE_URL=http://127.0.0.1:8005
EMBEDDING_BASE_URL=http://127.0.0.1:8006
LLM_BASE_URL=http://127.0.0.1:8007
OLLAMA_MODEL=llama3.2:3b
```

### Frontend Config (web/config.json)
```json
{
  "keycloak": {
    "issuer": "http://localhost:8180/realms/audio-transcriber",
    "clientId": "audio-translator-client",
    "scope": "openid profile email"
  },
  "services": {
    "asrBaseUrl": "http://localhost:8003",
    "translationBaseUrl": "http://localhost:8004",
    "ttsBaseUrl": "http://localhost:8005",
    "embeddingBaseUrl": "http://localhost:8006",
    "llmBaseUrl": "http://localhost:8007"
  }
}
```

### Whisper Model Selection
Edit `services/asr_py/app.py`:
```python
MODEL_NAME = "base"  # Options: tiny, base, small, medium, large
```

### Streaming Timing Configuration
Edit `cmd/server/main.go`:
```go
srv := session.NewServer(session.Config{
    PollInterval:  800 * time.Millisecond,  // ASR polling frequency
    WindowSeconds: 8,                        // Audio buffer size
    FinalizeAfter: 500 * time.Millisecond,  // Text stabilization time
})
```

### TTS Behavior (gTTS fallback + XTTS v2)
- The TTS service starts in **gTTS fallback** mode while XTTS v2 loads
- XTTS v2 enables higher quality and **voice cloning**
- If XTTS is unavailable, the system automatically falls back to gTTS

Check status:
```bash
curl http://127.0.0.1:8005/health
```

### Voice Cloning Notes
- XTTS v2 supports voice cloning for multiple languages
- Long text is chunked; if token limits are exceeded, it falls back to gTTS
- First run downloads ~1.8GB model (cached in Docker volume)

## 🔐 Keycloak Authentication

1. Create a realm (e.g. `audio-transcriber`)
2. Create a public client (e.g. `audio-translator-client`)
3. Set valid redirect URIs to `http://localhost:8080/*`
4. Set web origins to `http://localhost:8080`
5. Enable self-registration if desired
6. Update:
   - `KEYCLOAK_ISSUER` in `.env`
   - `web/config.json` `keycloak.issuer`

Meeting history and chat are account-scoped and require login.

## 🧾 Meeting Minutes + Backfill

Minutes are generated automatically after a meeting ends.

To backfill minutes for existing meetings:
```bash
# Requires LLM_BASE_URL and OLLAMA_MODEL in .env
go run cmd/backfill-minutes/main.go
```

## 🐛 Troubleshooting

### No audio is captured
- Check browser microphone permissions (click 🔒 in address bar)
- Open browser console (F12) and check for errors
- Ensure you're using HTTPS or localhost (WebRTC requirement)

### Database connection failed
- Ensure PostgreSQL container is running: `docker ps | grep postgres`
- Check `.env` file has correct credentials
- Verify migrations ran: `docker exec -it audio-translator-postgres-1 psql -U audio_translator -d audio_translator -c "\dt"`

### Meeting room not loading
- Check browser console for JavaScript errors
- Ensure server is loading `.env` variables
- Verify database tables exist (run migrations)

### Speaker diarization not working
- Add HuggingFace token to `.env` file
- Get token from: https://huggingface.co/settings/tokens
- Accept terms for pyannote models: https://huggingface.co/pyannote/speaker-diarization

### ASR service errors
- Check service is running: `curl http://localhost:8003/health`
- View logs: `docker compose logs asr -f`
- Verify Whisper model downloaded successfully

### TTS service slow
- First run downloads 1.8GB model (3-5 minutes)
- Falls back to gTTS while downloading
- Check status: `curl http://localhost:8005/health`

## 📊 Docker & Performance Notes

- Docker images run as non-root and use BuildKit cache mounts for faster rebuilds
- Resource limits are set in `docker-compose.yml` to avoid a single service starving the host
- ASR uses CUDA runtime images for smaller footprints

## 🧭 Roadmap (Production Hardening)

- HTTPS/TLS via reverse proxy
- API authentication + rate limiting
- Secrets management (vault or Docker secrets)
- Integration tests + structured logging
- Monitoring (Prometheus/Grafana)

## 🙏 Acknowledgments

- OpenAI Whisper for speech recognition
- XTTS v2 for text-to-speech
- Pyannote for speaker diarization
- Google Translate API for translations
