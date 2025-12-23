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

### Quick Start (All Services)

```bash
chmod +x start-all.sh
./start-all.sh
```

This starts all required services (ASR, Translation, and Go server).

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

**Voice Cloning Limitations:**
- Current implementation uses gTTS (Google Text-to-Speech) which provides standard voices
- True voice cloning requires advanced ML models:
  - **Coqui XTTS v2**: Requires Python 3.11 or older (Ubuntu 24.04 has Python 3.12)
  - **ElevenLabs API**: Requires paid API key and credits
  - **StyleTTS2**: Experimental and complex to set up

**Future Enhancements:**
To enable true voice cloning, you can:
1. Install Python 3.11 in a separate environment (using pyenv or conda)
2. Use ElevenLabs API with an API key (set `ELEVENLABS_API_KEY` environment variable)
3. Explore StyleTTS2 or other open-source alternatives

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

## Troubleshooting

### No audio is captured
- Check browser permissions for microphone access
- Look at browser console for errors (F12)

### ASR errors
- Ensure the Python service is running on port 8003
- Check the Python terminal for error messages
- Verify Whisper model downloaded successfully

### No translations
- Currently using stub translator (just adds language prefix)
- To add real translation, implement `internal/translate/translate.go`

## Technical Details

- Audio captured at browser's native sample rate (usually 48kHz)
- Resampled to 16kHz PCM16 in browser
- Sent via WebSocket as binary chunks
- Buffered in circular buffer on server
- Periodically sent to Whisper for transcription
- Results sent back to browser via WebSocket
