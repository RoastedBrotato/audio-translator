# Voice Cloning Setup Guide

This guide explains how to enable true voice cloning capabilities in the TTS service.

## Current Status

The TTS service currently uses **gTTS** (Google Text-to-Speech), which provides standard, non-personalized voices. The "Clone original voice" checkbox is present in the UI, but the backend does not perform actual voice cloning due to dependencies.

## Why Voice Cloning Isn't Currently Available

1. **Python Version Incompatibility**: The best open-source voice cloning library (Coqui XTTS v2) requires Python 3.11 or older
2. **System Has Python 3.12**: Ubuntu 24.04 comes with Python 3.12, which is incompatible
3. **Dependency Conflicts**: Alternative solutions like ElevenLabs SDK have pydantic version conflicts with FastAPI

## Options for Adding Voice Cloning

### Option 1: Install Python 3.11 (Recommended for XTTS v2)

Since Python 3.11 is not available in Ubuntu 24.04 repositories, you can use pyenv:

```bash
# Install pyenv
curl https://pyenv.run | bash

# Add to ~/.bashrc
export PATH="$HOME/.pyenv/bin:$PATH"
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"

# Install Python 3.11
pyenv install 3.11.9

# Create venv with Python 3.11
cd services/tts_py
pyenv local 3.11.9
python -m venv venv
source venv/bin/activate

# Install XTTS dependencies
pip install fastapi uvicorn python-multipart
pip install TTS torch torchaudio numpy

# Update app.py to use XTTS (see Option 1 code below)
```

**app.py for XTTS v2:**

```python
#!/usr/bin/env python3
from fastapi import FastAPI, File, UploadFile, Form, HTTPException
from fastapi.responses import Response
from TTS.api import TTS
import tempfile
import logging
import os

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="TTS Service with XTTS v2")

# Initialize XTTS model
tts_model = None

@app.on_event("startup")
async def load_model():
    global tts_model
    logger.info("Loading XTTS v2 model...")
    tts_model = TTS("tts_models/multilingual/multi-dataset/xtts_v2")
    logger.info("XTTS v2 model loaded!")

@app.post("/synthesize_with_voice")
async def synthesize_with_voice(
    text: str = Form(...),
    language: str = Form("en"),
    reference_audio: UploadFile = File(...)
):
    """Voice cloning with XTTS v2"""
    try:
        # Save reference audio to temp file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as ref_file:
            ref_file.write(await reference_audio.read())
            ref_audio_path = ref_file.name
        
        # Generate speech with voice cloning
        output_path = tempfile.mktemp(suffix=".wav")
        
        tts_model.tts_to_file(
            text=text,
            file_path=output_path,
            speaker_wav=ref_audio_path,
            language=language
        )
        
        # Read output
        with open(output_path, "rb") as f:
            audio_data = f.read()
        
        # Cleanup
        os.unlink(ref_audio_path)
        os.unlink(output_path)
        
        return Response(
            content=audio_data,
            media_type="audio/wav",
            headers={"Content-Disposition": "inline"}
        )
    except Exception as e:
        logger.error(f"Voice cloning error: {e}")
        raise HTTPException(status_code=500, detail=str(e))
```

### Option 2: Use ElevenLabs API (Commercial Service)

ElevenLabs provides high-quality voice cloning as a paid service.

```bash
# Get API key from https://elevenlabs.io/
export ELEVENLABS_API_KEY="your-api-key-here"

# Install (use compatible versions)
pip install elevenlabs==1.5.0 fastapi python-multipart
```

**app.py for ElevenLabs:**

```python
#!/usr/bin/env python3
from fastapi import FastAPI, File, UploadFile, Form, HTTPException
from fastapi.responses import Response
from elevenlabs.client import ElevenLabs
from elevenlabs import VoiceSettings
import os
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="TTS Service with ElevenLabs")

@app.post("/synthesize_with_voice")
async def synthesize_with_voice(
    text: str = Form(...),
    language: str = Form("en"),
    reference_audio: UploadFile = File(...)
):
    """Voice cloning with ElevenLabs"""
    try:
        api_key = os.getenv("ELEVENLABS_API_KEY")
        if not api_key:
            raise HTTPException(status_code=500, detail="ELEVENLABS_API_KEY not set")
        
        client = ElevenLabs(api_key=api_key)
        
        # Read reference audio
        reference_bytes = await reference_audio.read()
        
        # Use voice isolation for best quality
        audio_generator = client.text_to_speech.convert_with_voice_isolation(
            text=text,
            audio=reference_bytes,
            voice_settings=VoiceSettings(
                stability=0.5,
                similarity_boost=0.75,
                use_speaker_boost=True
            )
        )
        
        # Collect audio chunks
        audio_bytes = b''.join(chunk for chunk in audio_generator if chunk)
        
        return Response(
            content=audio_bytes,
            media_type="audio/mpeg",
            headers={"Content-Disposition": "inline"}
        )
    except Exception as e:
        logger.error(f"ElevenLabs error: {e}")
        raise HTTPException(status_code=500, detail=str(e))
```

### Option 3: Use Docker with Python 3.11

Create a Dockerfile for the TTS service:

```dockerfile
FROM python:3.11-slim

WORKDIR /app

# Install ffmpeg for audio processing
RUN apt-get update && apt-get install -y ffmpeg && rm -rf /var/lib/apt/lists/*

# Copy requirements
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Install XTTS
RUN pip install --no-cache-dir TTS torch torchaudio

# Copy app
COPY app.py .

# Expose port
EXPOSE 8005

# Run
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8005"]
```

Run with:
```bash
docker build -t tts-service services/tts_py/
docker run -p 8005:8005 tts-service
```

### Option 4: Use StyleTTS2 (Experimental)

StyleTTS2 is a newer open-source alternative, but requires more setup:

```bash
# Clone StyleTTS2
git clone https://github.com/yl4579/StyleTTS2.git
cd StyleTTS2

# Follow their setup instructions
# Wrap in FastAPI service similar to XTTS
```

## Testing Voice Cloning

Once you've implemented one of the options above:

1. Start the TTS service
2. Go to http://localhost:8080/video.html
3. Upload a video
4. Check both "Generate translated audio" AND "Clone original voice"
5. Process the video
6. The resulting audio should match the original speaker's voice

## Performance Considerations

- **XTTS v2**: ~5-10 seconds per sentence on CPU, ~1-2 seconds on GPU
- **ElevenLabs**: Fast (cloud-based), but costs money per character
- **gTTS** (current): Fast, free, but no voice cloning

## Cost Considerations

- **XTTS v2**: Free, open-source, runs locally
- **ElevenLabs**: Paid service (~$0.0003 per character)
- **gTTS**: Free

## Recommendation

For the best balance of quality and cost:
1. Use **pyenv** to install Python 3.11
2. Install **XTTS v2** with the code from Option 1
3. This gives you free, high-quality voice cloning running locally

For production or if you need the absolute best quality:
- Use **ElevenLabs API** (Option 2) with appropriate API key management
