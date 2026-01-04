# Real-time Audio Translator

A comprehensive real-time speech-to-text and translation system with support for live streaming, meeting rooms, video/audio processing, and multi-user collaboration.

## 🌟 Features

### Live Translation Modes
- **🎙️ Real-time Streaming**: Live microphone transcription with instant translation
- **📹 Meeting Rooms**: Multi-user meetings with real-time translation for each participant
  - **Individual Device Mode**: Each person joins with their own microphone
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

## 🏗️ Architecture

### System Components
- **Frontend**: Modern web interface with feature-based architecture
- **Backend**: Go server with WebSocket support and REST API
- **ASR Service**: Python FastAPI + Whisper for speech recognition
- **Translation Service**: Python service using Google Translate API
- **TTS Service**: XTTS v2 for text-to-speech with voice cloning
- **Database**: PostgreSQL for meeting data, participants, and speaker profiles
- **Video Processing**: FFmpeg for audio/video manipulation

### Web Directory Structure
```
web/
├── index.html                    # Landing page
├── assets/                       # Shared resources
│   ├── css/                      # Modular stylesheets
│   │   ├── variables.css         # Design tokens (colors, spacing)
│   │   ├── base.css              # Base styles
│   │   ├── buttons.css           # Button components
│   │   ├── forms.css             # Form elements
│   │   ├── layout.css            # Page layouts
│   │   └── ... (15+ modular CSS files)
│   ├── js/                       # Shared utilities
│   │   ├── audio-processor.js    # Audio conversion & processing
│   │   ├── utils.js              # Helper functions (escapeHtml, debounce, etc.)
│   │   └── websocket-manager.js  # WebSocket wrapper
│   └── images/                   # Static images
├── components/                   # Reusable UI components
│   └── navbar/                   # Navigation bar component
│       └── navbar.js
└── features/                     # Feature modules
    ├── home/                     # Home page
    ├── streaming/                # Live streaming feature
    │   ├── streaming.html
    │   ├── streaming.js
    │   └── pcm-worklet.js
    ├── recording/                # Audio upload & processing
    │   ├── recording.html
    │   └── recording.js
    ├── video/                    # Video upload & processing
    │   ├── video.html
    │   └── video.js
    └── meeting/                  # Meeting rooms
        ├── meeting-create.html   # Create/join meetings
        ├── meeting-join.html     # Join with name & language
        ├── meeting-room.html     # Active meeting interface
        └── meeting-room.js       # Meeting room logic (ES6 modules)
```

### Shared Utilities

All feature modules import shared utilities to eliminate code duplication:

**audio-processor.js**
```javascript
import { convertToPCM16, getAudioLevel, samplesToWAV } from '/assets/js/audio-processor.js';
```
- `convertToPCM16()` - Float32 to PCM16 conversion
- `getAudioLevel()` - Calculate audio levels for meters
- `samplesToWAV()` - Generate WAV file headers
- `hasVoiceActivity()` - Voice activity detection

**utils.js**
```javascript
import { escapeHtml, getLanguageName, debounce, formatTimestamp } from '/assets/js/utils.js';
```
- `escapeHtml()` - XSS prevention for user-generated content
- `getLanguageName()` - Language code to display name
- `debounce()` - Debounce function calls
- `formatTimestamp()` - Human-readable timestamps
- `downloadBlob()` - Trigger file downloads

## 📋 Prerequisites

- **Go** 1.16+ (for backend server)
- **Python** 3.8+ (for AI services)
- **Docker & Docker Compose** (recommended deployment)
- **FFmpeg** (for video/audio processing)
- **PostgreSQL** 15+ (for meeting data persistence)

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
- ✅ Starts PostgreSQL database
- ✅ Starts ASR, Translation, and TTS services
- ✅ Runs database migrations
- ✅ Builds and starts the Go web server
- ✅ Shows service URLs and status

**First-time setup:**
- Docker images take 15-30 minutes to build (one time)
- ASR downloads Whisper model (~500MB)
- TTS downloads XTTS v2 model (~1.8GB)
- Models are cached in Docker volumes

**Service URLs:**
- 🌐 Web UI: http://localhost:8080
- 🎤 ASR Service: http://localhost:8003
- 🌍 Translation: http://localhost:8004
- 🔊 TTS Service: http://localhost:8005

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

**Live microphone translation:**

1. Navigate to http://localhost:8080
2. Click **"Streaming Translation"**
3. Select source and target languages
4. Click **Start** and grant microphone permission
5. Speak into your microphone
6. See real-time transcription and translation
7. Download transcript when done

### 2. Meeting Rooms

**Multi-user meetings with translation:**

#### Create a Meeting (Individual Device Mode)
1. Go to http://localhost:8080/meeting.html
2. Click **"Individual Devices"** mode
3. Copy the room code (e.g., `ABC-123`)
4. Share the code with participants
5. Click **Join Meeting**
6. Enter your name and select your language
7. Grant microphone permission
8. Start speaking!

**How it works:**
- Each participant joins from their own device
- Everyone's audio is transcribed separately
- Translations appear in each person's preferred language
- Speaker labels are customizable (click to rename)

#### Create a Meeting (Shared Room Mode)
1. Click **"Shared Room"** mode instead
2. Multiple people speak into the same microphone
3. AI automatically identifies and labels speakers
4. Each participant still sees translations in their language

**Features:**
- Real-time participant list
- Live captions in your language
- Click speaker labels to rename them
- Download transcript in any language
- Host can end the meeting for everyone

### 3. Video Translation

**Upload and translate videos:**

1. Go to http://localhost:8080/video.html
2. Drag and drop a video file (or click to browse)
3. Select source language (or auto-detect)
4. Select target language
5. **Optional**: Check "Generate translated audio"
6. **Optional**: Check "Clone original voice" (experimental)
7. Click **Process Video**
8. Wait for processing (progress shown)
9. Download transcript or translated video

**Supported formats:** MP4, AVI, MOV, MKV, WebM (max 500MB)

### 4. Audio Recording

**Upload audio files with speaker diarization:**

1. Go to http://localhost:8080/recording.html
2. Upload an audio file
3. Enable **"Speaker Diarization"** for multi-speaker audio
4. Enable **"Audio Enhancement"** for noisy recordings
5. Process and view results with speaker-labeled segments

## 🔧 Configuration

### Environment Variables (.env)

```bash
# Database Configuration
DB_HOST=localhost
DB_PORT=5433
DB_USER=audio_translator
DB_PASSWORD=audio_translator_pass
DB_NAME=audio_translator

# HuggingFace Token (required for speaker diarization)
HF_TOKEN=your_token_here

# CORS (optional - leave empty for development)
ALLOWED_ORIGINS=

# Speaker Diarization Tuning
SPEAKER_SIM_THRESHOLD=0.82                     # Embedding similarity threshold
MIN_EMBED_DURATION=0.8                          # Min seconds for speaker embedding
SPEAKER_OVERLAP_RATIO_THRESHOLD=0.25            # Overlap detection threshold
SPEAKER_CONFIDENCE_THRESHOLD=0.55               # Min confidence for speaker label
SPEAKER_PROFILE_TTL_SECONDS=3600                # Idle time before profile expires
SPEAKER_PROFILE_PERSIST_INTERVAL_SECONDS=15     # Persistence interval

# Speaker Profile Database Cleanup
SPEAKER_PROFILE_DB_TTL_SECONDS=86400            # Delete profiles older than 24h
SPEAKER_PROFILE_DB_CLEANUP_INTERVAL_SECONDS=300 # Run cleanup every 5 minutes
```

### Whisper Model Selection

Edit `services/asr_py/app.py`:
```python
MODEL_NAME = "base"  # Options: tiny, base, small, medium, large
```

**Model sizes:**
- `tiny` - Fastest, less accurate (~40MB)
- `base` - Fast, good for real-time (~75MB) **[Default]**
- `small` - Better accuracy (~244MB)
- `medium` - High accuracy, slower (~769MB)
- `large` - Best accuracy, very slow (~1.5GB)

### Server Timing Configuration

Edit `cmd/server/main.go`:
```go
srv := session.NewServer(session.Config{
    ASRBaseURL:    "http://127.0.0.1:8003",
    PollInterval:  800 * time.Millisecond,  // ASR polling frequency
    WindowSeconds: 8,                        // Audio buffer size
    FinalizeAfter: 500 * time.Millisecond,  // Text stabilization time
})
```

## 🛠️ Development Guide

### Adding a New Feature

Follow the feature-based architecture pattern:

1. **Create feature directory:**
   ```bash
   mkdir -p web/features/myfeature
   ```

2. **Create HTML file:**
   ```html
   <!-- web/features/myfeature/myfeature.html -->
   <!DOCTYPE html>
   <html>
   <head>
       <link rel="stylesheet" href="../../assets/css/styles.css">
   </head>
   <body>
       <!-- Your UI here -->
       <script type="module" src="myfeature.js"></script>
   </body>
   </html>
   ```

3. **Create JavaScript module:**
   ```javascript
   // web/features/myfeature/myfeature.js
   import { convertToPCM16 } from '/assets/js/audio-processor.js';
   import { escapeHtml } from '/assets/js/utils.js';

   // Your feature logic here
   ```

4. **Add route redirect in Go server:**
   ```go
   // cmd/server/main.go
   http.HandleFunc("/myfeature.html", func(w http.ResponseWriter, r *http.Request) {
       http.ServeFile(w, r, "./web/features/myfeature/myfeature.html")
   })
   ```

5. **Rebuild and test:**
   ```bash
   go build -o bin/server cmd/server/main.go
   ./start-services.sh
   ```

### Code Style Guidelines

**JavaScript:**
- Use ES6 modules (`import`/`export`)
- Absolute paths for imports (`/assets/js/...`)
- Always escape user-generated content with `escapeHtml()`
- Use event delegation for dynamic elements
- Store data in `data-*` attributes, not inline handlers

**CSS:**
- Use CSS variables from `variables.css`
- Create new modular CSS files for feature-specific styles
- Import in `styles.css` using `@import`

**Go:**
- Follow standard Go conventions
- Use structured logging
- Add error handling for all external calls
- Use environment variables for configuration

### Database Migrations

Add new migrations in `migrations/` directory:

```sql
-- migrations/006_your_feature.sql
CREATE TABLE IF NOT EXISTS your_table (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP DEFAULT NOW()
);
```

Apply manually:
```bash
cat migrations/006_your_feature.sql | docker exec -i audio-translator-postgres-1 psql -U audio_translator -d audio_translator
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

## 📊 Performance Tips

### For Real-time Streaming
- Use `base` or `tiny` Whisper model for faster response
- Reduce `PollInterval` to 500ms for quicker updates
- Decrease `WindowSeconds` to 5-6 for shorter latency

### For Accuracy
- Use `small` or `medium` Whisper model
- Increase `FinalizeAfter` to 900ms to reduce partial updates
- Enable audio enhancement for noisy recordings

### For Meetings
- Individual device mode has lower latency than shared room
- Limit participants to 10-15 for best performance
- Use wired internet connection when possible

## 🤝 Contributing

Contributions are welcome! Please:

1. Follow the existing code structure
2. Use shared utilities instead of duplicating code
3. Add comments for complex logic
4. Test thoroughly before submitting
5. Update documentation for new features

## 🙏 Acknowledgments

- OpenAI Whisper for speech recognition
- XTTS v2 for text-to-speech
- Pyannote for speaker diarization
- Google Translate API for translations

---