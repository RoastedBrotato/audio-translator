# Realtime Caption Translator

A real-time speech-to-text and translation system that captures audio from your microphone, transcribes it using OpenAI Whisper, and provides translations. Now includes video upload and batch translation support!

## Features

- **Real-time Translation**: Live microphone audio transcription and translation
- **Video Translation**: Upload videos to extract audio, transcribe, and translate
- Support for multiple languages (English, Arabic, Spanish, French, German, Chinese, Japanese)
- Modern web interface with progress tracking

## Architecture

- **Frontend**: Web interface (HTML/JS) with WebSocket for real-time and REST API for video uploads
- **Backend**: Go server handling WebSocket connections, audio buffering, and video processing
- **ASR Service**: Python FastAPI service running Whisper for speech recognition
- **Translation Service**: Python service for text translation
- **Video Processing**: FFmpeg integration for audio extraction from video files

## Prerequisites

- Go 1.16+
- Python 3.8+
- FFmpeg (for video processing)

Install FFmpeg:
```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg
```

## Setup & Running

### Quick Start with Docker (Recommended)

The easiest way to run all services with proper dependencies:

```bash
# Start all Docker services
docker compose up -d

# Start the Go server
go run cmd/server/main.go
```

Open http://localhost:8080 in your browser.

**First-time setup notes:**
- TTS service downloads XTTS v2 model (~1.8GB) on first run - takes 3-5 minutes
- Service works immediately with gTTS fallback, voice cloning available after download completes
- Model is cached in Docker volume for future use

**Useful commands:**
```bash
# Check service status
docker compose ps

# View logs
docker compose logs -f

# Check TTS status specifically
curl http://127.0.0.1:8005/health

# Stop services
docker compose down
```

See [DOCKER_SETUP.md](DOCKER_SETUP.md) for detailed Docker instructions.

### Quick Start (Native - All Services)

```bash
chmod +x start-all.sh
./start-all.sh
```

This starts all required services (ASR, Translation, TTS, and Go server). 

**Note**: Native TTS service requires Python 3.11 and will need to download XTTS v2 model.

### Manual Setup

### 1. Start the ASR Service (Terminal 1)

```bash
chmod +x start-asr.sh
./start-asr.sh
```

This will:
- Install Python dependencies (fastapi, uvicorn, whisper, numpy, torch)
- Start the ASR service on http://127.0.0.1:8003

**Note**: First run will download the Whisper model (~40MB for "tiny" model).

### 2. Start the Translation Service (Terminal 2)

```bash
chmod +x start-translate.sh
./start-translate.sh
```

This starts the translation service on http://127.0.0.1:8004

### 3. Start the Go Server (Terminal 3)

```bash
chmod +x start-server.sh
./start-server.sh
```

This starts the WebSocket server on http://localhost:8080

### 4. Open the Web Interface

Open your browser and navigate to:
```
http://localhost:8080
```

## Usage

### Real-time Translation

1. Go to http://localhost:8080
2. Click **Start** to begin capturing audio
3. Speak into your microphone
4. See transcriptions appear in the "Original" box
5. See translations appear in the "Translation" box
6. Click **Stop** when done

### Video Translation

1. Go to http://localhost:8080/video.html (or click the link on the main page)
2. Click the upload area or drag and drop a video file
3. Select source and target languages
4. **Optional**: Check "Generate translated audio" to replace the original audio with TTS
5. **Optional**: Check "Clone original voice" to attempt voice preservation (see limitations below)
6. Click **Process Video**
7. Wait for the video to be processed
8. View transcription and translation results
9. If TTS was enabled, click **Download Video with Translated Audio** to get your video

**Features:**
- Text transcription and translation
- Optional TTS (Text-to-Speech) generation
- Audio replacement - replace original audio with translated speech
- Download processed video with translated audio

Supported video formats: MP4, AVI, MOV, MKV, WebM (max 500MB)

## Configuration

### Change Whisper Model

Edit `services/asr_py/app.py` line 13:
- `tiny` - Fastest, less accurate (~40MB)
- `base` - Fast, better accuracy (~75MB)
- `small` - Good balance (~244MB)
- `medium` - Better but slower (~769MB)

### Change Target Language

Use the dropdown in the web interface to select:
- Arabic (ar)
- English (en)
- French (fr)
- Spanish (es)

### Adjust Timing

Edit `cmd/server/main.go` to tune:
- `PollInterval` - How often to check for new transcriptions (default: 800ms)
- `WindowSeconds` - Audio buffer size (default: 8 seconds)
- `FinalizeAfter` - How long text must be stable before finalizing (default: 500ms)

### Speaker Diarization Tuning

Set these environment variables (see `.env.example`) to tune diarization accuracy:
- `SPEAKER_SIM_THRESHOLD` (default: `0.82`) - embedding similarity to keep a persistent speaker ID
- `MIN_EMBED_DURATION` (default: `0.8`) - minimum seconds of audio needed to compute an embedding
- `SPEAKER_OVERLAP_RATIO_THRESHOLD` (default: `0.25`) - overlap ratio to flag segments as overlapping
- `SPEAKER_CONFIDENCE_THRESHOLD` (default: `0.55`) - minimum overlap ratio to consider a speaker label confident
- `SPEAKER_PROFILE_TTL_SECONDS` (default: `3600`) - idle time before speaker profiles expire
- `SPEAKER_PROFILE_CLEANUP_INTERVAL_SECONDS` (default: `300`) - how often to sweep expired profiles
- `SPEAKER_PROFILE_STORE_URL` (optional) - Go server base URL for persisting speaker profiles
- `SPEAKER_PROFILE_PERSIST_INTERVAL_SECONDS` (default: `15`) - minimum seconds between persistence updates
- `SPEAKER_PROFILE_DB_TTL_SECONDS` (optional) - delete stored profiles older than this (Go server cleanup)
- `SPEAKER_PROFILE_DB_CLEANUP_INTERVAL_SECONDS` (default: `300`) - cleanup cadence for stored profiles

### Speaker Profile Cleanup Script

Run manual cleanup against the Go server:
```bash
./cleanup-speaker-profiles.sh 86400
```

Optional environment variables:
- `BASE_URL` (default: `http://localhost:8080`)
- `SPEAKER_PROFILE_DB_TTL_SECONDS` (used if no arg provided)

## Troubleshooting

### No audio is captured
- Check browser permissions for microphone access
- Look at browser console for errors (F12)

### ASR errors
- Ensure the Python service is running on port 8003
- Check the Python terminal for error messages
- Verify Whisper model downloaded successfully

## Technical Details

- Audio captured at browser's native sample rate (usually 48kHz)
- Resampled to 16kHz PCM16 in browser
- Sent via WebSocket as binary chunks
- Buffered in circular buffer on server
- Periodically sent to Whisper for transcription
- Results sent back to browser via WebSocket
