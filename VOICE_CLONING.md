# Voice Cloning with XTTS v2

The TTS service now includes **working voice cloning** capabilities using Coqui XTTS v2, running in a Docker container with Python 3.11.

## Current Status

✅ **Voice cloning is ACTIVE** for the video upload feature
✅ Uses Coqui XTTS v2 (multilingual, open-source)
✅ Automatic text chunking for long translations
✅ Graceful fallback to gTTS for unsupported scenarios

## How It Works

### Architecture

The TTS service (`services/tts_py/`) runs in Docker with:
- Python 3.11 (required for XTTS v2)
- Coqui TTS library with XTTS v2 model (1.87 GB)
- Automatic model download on first container build
- Background loading on startup (service available immediately with gTTS)

### Endpoints

**`POST /synthesize_with_voice`** - Voice cloning with reference audio
- Parameters:
  - `text` (string): Text to synthesize
  - `language` (string): Target language code (e.g., "en", "hi", "es")
  - `reference_audio` (file): WAV file of original speaker's voice
- Returns: WAV audio with cloned voice
- Falls back to standard TTS if XTTS fails

**`POST /synthesize`** - Standard TTS without voice cloning
- Parameters:
  - `text` (string): Text to synthesize
  - `language` (string): Target language code
- Returns: WAV audio with standard voice (gTTS or XTTS)

**`GET /health`** - Service health check
- Returns model loading status and available features

## Supported Languages

XTTS v2 supports voice cloning in these languages:
- English (en)
- Spanish (es)
- French (fr)
- German (de)
- Italian (it)
- Portuguese (pt)
- Polish (pl)
- Turkish (tr)
- Russian (ru)
- Dutch (nl)
- Czech (cs)
- Arabic (ar)
- Chinese (zh)
- Hungarian (hu)
- Korean (ko)
- Japanese (ja)
- Hindi (hi)

## Known Limitations

### Token Limit
XTTS v2 has a **400 token limit** per synthesis request. The service handles this with:

1. **Text Chunking**: Long texts are automatically split into ~250 character chunks
2. **Chunk Processing**: Each chunk is synthesized separately with voice cloning
3. **Audio Stitching**: Chunks are combined into a single output file

**Current Issue**: For some languages like Hindi, even 250-character chunks can exceed the 400 token limit because:
- Tokens ≠ Characters
- Non-Latin scripts may use more tokens per character

**Behavior**: When token limit is exceeded, the system automatically falls back to gTTS (standard TTS without voice cloning)

### Fallback Scenarios

The service falls back to gTTS in these cases:
1. XTTS v2 model is still loading (during initial startup)
2. Text exceeds 400 token limit (even after chunking)
3. XTTS v2 encounters an error
4. Language is not supported by XTTS v2

## Usage

### Via Video Upload Page

1. Go to `http://localhost:8080/video.html`
2. Upload a video file
3. Select source and target languages
4. ✅ Check "Generate translated audio"
5. ✅ Check "Clone voice from original audio"
6. Click "Upload and Process"

The system will:
- Extract audio from the video
- Transcribe the speech
- Translate to target language
- Generate dubbed audio with the original speaker's voice
- Replace audio in the video

### Via API

```bash
# Voice cloning
curl -X POST http://localhost:8005/synthesize_with_voice \
  -F "text=नमस्ते, यह एक परीक्षण है" \
  -F "language=hi" \
  -F "reference_audio=@original_speaker.wav" \
  -o cloned_voice.wav

# Standard TTS
curl -X POST http://localhost:8005/synthesize \
  -H "Content-Type: application/json" \
  -d '{"text": "Hello, this is a test", "language": "en"}' \
  -o standard_voice.wav
```

## Performance

- **Model Loading**: 2-5 minutes on first container start (downloads 1.87 GB)
- **Synthesis Speed**: ~5-10 seconds per sentence on CPU, ~1-2 seconds on GPU
- **Fallback (gTTS)**: < 1 second per sentence (cloud-based)

## Setup

Voice cloning is automatically configured when you build the Docker container:

```bash
# Start all services with voice cloning
./start-streaming.sh
```

The TTS container will:
1. Download XTTS v2 model (~1.87 GB) on first build
2. Load the model in background on startup
3. Make the service available immediately with gTTS
4. Switch to XTTS v2 when loading completes

## Troubleshooting

### "XTTS v2 model is still loading"

The model is downloading or loading. This happens on:
- First container start (downloads ~1.87 GB)
- Container rebuild (re-downloads model)

**Solution**: Wait 2-5 minutes for the model to load. Check status:
```bash
docker logs audio-translator-tts 2>&1 | grep "XTTS"
```

Look for: `✓ XTTS v2 model loaded successfully!`

### Voice cloning falls back to standard TTS

This can happen for several reasons:

1. **Long text**: Exceeds 400 token limit even after chunking
   - **Solution**: Use shorter segments, or accept gTTS fallback

2. **Unsupported language**: Language not in XTTS v2's supported list
   - **Solution**: Check supported languages above

3. **Model not loaded**: XTTS v2 still loading
   - **Solution**: Wait for model to finish loading

### Container keeps re-downloading the model

The model downloads every time the container is rebuilt. To persist:

```bash
# Create a volume for model cache
docker volume create tts-models

# Add to docker run command in start-streaming.sh
-v tts-models:/root/.local/share/tts
```

## Cost Comparison

| Solution | Cost | Quality | Speed | Voice Cloning |
|----------|------|---------|-------|---------------|
| XTTS v2 (current) | Free | High | Medium | Yes |
| gTTS (fallback) | Free | Medium | Fast | No |
| ElevenLabs | ~$0.30/1000 chars | Excellent | Very Fast | Yes |

## Future Improvements

Potential enhancements:
1. **Token-based chunking**: Use XTTS tokenizer instead of character count
2. **GPU acceleration**: Add GPU support for faster synthesis
3. **Model persistence**: Cache model in Docker volume
4. **Alternative models**: Support for other voice cloning models
5. **Quality tuning**: Expose XTTS parameters for voice quality adjustment
