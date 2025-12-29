# Streaming Caption System Architecture

## Overview
Real-time streaming captions with low latency + high-quality final export.

## Architecture

### 1. Live Streaming Phase
```
Browser Mic → WebSocket (16kHz PCM) → Faster-Whisper (GPU) → Live Captions
                                              ↓
                                         Silero VAD
                                              ↓
                                     Partial Transcripts (shown live)
                                              ↓
                                     Final Segments (on silence)
                                              ↓
                                        Translation API
                                              ↓
                                    Translated Segments (displayed)
```

### 2. Final Export Phase (TODO)
```
Full Audio Recording → High-Quality Whisper (large model) → Full Transcript
                                                                   ↓
                                                            Translation API
                                                                   ↓
                                                            Formatted Document
```

## Components

### Streaming ASR Service (Port 8003)
- **Location**: `services/asr_streaming/`
- **Technology**: faster-whisper + Silero VAD
- **GPU**: CUDA-accelerated (RTX 4060)
- **Model**: Base (fast, low latency)
- **Features**:
  - Real-time streaming transcription
  - VAD-based segment detection
  - Partial and final transcripts
  - 1-second chunk processing

### Translation Service (Port 8004)
- **Location**: `services/translate_py/`
- **Existing service**: deep-translator with Google Translate
- **Translates finalized segments only**

### Go Server (Port 8080)
- **Location**: `cmd/server/main.go`
- **Serves**: Web frontend
- **Routes**: `/streaming.html`, static files

### Frontend
- **File**: `web/streaming.html`, `web/streaming.js`
- **Features**:
  - Live caption display (partial/final)
  - Finalized segment translations
  - Statistics (segments, translations, duration)
  - Export options (text, JSON)

## Key Features

### 1. VAD (Voice Activity Detection)
- **Model**: Silero VAD
- **Threshold**: 0.5 (50% speech probability)
- **Silence Duration**: 1.5 seconds to finalize segment
- **Benefit**: Don't process silence, save GPU/CPU

### 2. Streaming Transcription
- **Chunk Duration**: 1 second
- **Transcription Interval**: Minimum 500ms between updates
- **Partial Results**: Shown immediately (gray, italic)
- **Final Results**: Shown on silence detection (bold)

### 3. Smart Translation
- **Only translate finalized segments** (not partials)
- **Async translation** doesn't block streaming
- **Fallback**: Original text if translation fails

### 4. GPU Acceleration
- **Device**: CUDA (RTX 4060)
- **Compute Type**: float16 (faster on GPU)
- **Speed**: ~10-20x faster than CPU medium model

## Usage

### Start Services
```bash
./start-streaming.sh
```

### Access
```
http://localhost:8080/streaming.html
```

### Workflow
1. Select source language (Urdu) and target language (English)
2. Click "Start Streaming"
3. Speak → See live captions appear in real-time
4. After 1.5s silence → Segment finalizes → Gets translated
5. Click "Stop & Export" → Download transcript

## Next Steps (TODO)

### 1. Final High-Quality Processing
```python
# On Stop button:
1. Save full audio recording
2. Run Whisper large model on full audio
3. Get complete high-quality transcript
4. Translate entire transcript (better context)
5. Format into professional document (PDF/DOCX)
6. Offer download
```

### 2. Improvements
- [ ] Punctuation and capitalization
- [x] Speaker diarization (IMPLEMENTED - see `recording.html`)
- [ ] Keyword highlighting
- [ ] Configurable VAD threshold
- [ ] Configurable silence duration
- [ ] Custom model selection
- [ ] Export to PDF/DOCX with formatting
- [ ] Audio level visualization
- [ ] Pause/Resume functionality

## Performance

### Current (Streaming)
- **Latency**: 500ms - 1.5s (partial)
- **Finalization**: 1.5s after silence
- **Model**: Base (fastest)
- **GPU Usage**: ~2GB VRAM
- **Accuracy**: Good for live captions

### Final Export (Planned)
- **Processing Time**: 30-60s for 5min audio
- **Model**: Large (best accuracy)
- **GPU Usage**: ~6GB VRAM
- **Accuracy**: Excellent for documents

## Files Structure

```
services/
  asr_streaming/
    app.py              # Streaming ASR with VAD
    requirements.txt    # faster-whisper, torch, silero-vad
    Dockerfile          # CUDA base image
    
web/
  streaming.html        # Live caption UI
  streaming.js          # WebSocket client, translation logic
  
cmd/server/main.go      # Updated with /ws/stream endpoint

start-streaming.sh      # Startup script with GPU support
```

## Dependencies

### Python (ASR Streaming)
- faster-whisper 1.0.3 (GPU-accelerated Whisper)
- torch 2.1.0 + CUDA 12.0
- silero-vad (voice activity detection)
- fastapi + uvicorn + websockets

### Docker
- nvidia/cuda:12.0.0-cudnn8-runtime-ubuntu22.04
- GPU support via `--gpus all`

## Configuration

### Model Sizes
- **tiny**: 40MB, fastest, lowest accuracy
- **base**: 74MB, fast, good accuracy ← Current
- **small**: 244MB, slower, better accuracy
- **medium**: 1.5GB, slow, high accuracy
- **large**: 3GB, slowest, best accuracy ← For final export

### VAD Settings
- `VAD_THRESHOLD = 0.5`: Speech detection sensitivity
- `SILENCE_DURATION = 1.5`: Seconds to wait before finalizing
- `CHUNK_DURATION = 1.0`: Audio chunk size for processing

## Troubleshooting

### GPU Not Detected
```bash
# Check GPU
nvidia-smi

# Check Docker GPU access
docker run --rm --gpus all nvidia/cuda:12.0.0-base-ubuntu22.04 nvidia-smi
```

### Service Not Starting
```bash
# Check logs
docker logs -f asr-streaming
tail -f /tmp/go-server.log

# Rebuild
cd services/asr_streaming
docker build -t asr-streaming:latest .
docker stop asr-streaming && docker rm asr-streaming
docker run -d --name asr-streaming --gpus all -p 8003:8003 asr-streaming:latest
```

### No Captions Appearing
1. Check browser console for WebSocket errors
2. Verify mic permissions granted
3. Check ASR service health: `curl http://localhost:8003/health`
4. Adjust VAD threshold if speech not detected

### Poor Translation Quality
- Wait for finalized segments (not partials)
- Use better source transcription (upgrade model size)
- For final export, use full-audio re-transcription

## Performance Monitoring

### Check GPU Usage
```bash
watch -n 1 nvidia-smi
```

### Check Service Health
```bash
curl http://localhost:8003/health
# Response: {"status":"ok","device":"cuda","model":"base"}
```

### Monitor Logs
```bash
# ASR Service
docker logs -f asr-streaming

# Go Server
tail -f /tmp/go-server.log
```
