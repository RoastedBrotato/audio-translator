#!/usr/bin/env python3
"""
TTS Service with Coqui XTTS v2 Voice Cloning
Provides high-quality voice cloning from reference audio.
"""
from fastapi import FastAPI, File, UploadFile, Form, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import Response
from pydantic import BaseModel
from TTS.api import TTS
from gtts import gTTS
import tempfile
import os
import logging
import asyncio
from threading import Thread
import re
from pydub import AudioSegment

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="TTS Service with XTTS v2")

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Global TTS model
tts_model = None
model_loading = True

def load_xtts_model():
    """Load XTTS v2 model in background thread"""
    global tts_model, model_loading
    try:
        logger.info("Loading XTTS v2 model in background... This may take a few minutes...")
        tts_model = TTS("tts_models/multilingual/multi-dataset/xtts_v2")
        logger.info("✓ XTTS v2 model loaded successfully!")
    except Exception as e:
        logger.warning(f"XTTS v2 not available: {e}")
        logger.info("Service will continue using gTTS fallback")
    finally:
        model_loading = False

def chunk_text(text, max_chars=250):
    """
    Split text into chunks suitable for XTTS v2 voice cloning.
    XTTS has a limit of ~400 tokens, so we keep chunks around 250 characters
    to be safe and split on sentence boundaries.
    """
    # Split on sentence boundaries
    sentences = re.split(r'(?<=[.!?])\s+', text)

    chunks = []
    current_chunk = ""

    for sentence in sentences:
        # If adding this sentence exceeds max_chars, save current chunk and start new one
        if len(current_chunk) + len(sentence) > max_chars and current_chunk:
            chunks.append(current_chunk.strip())
            current_chunk = sentence
        else:
            current_chunk += (" " if current_chunk else "") + sentence

    # Add the last chunk if any
    if current_chunk:
        chunks.append(current_chunk.strip())

    return chunks if chunks else [text]

@app.on_event("startup")
async def startup_event():
    """Start model loading in background"""
    logger.info("TTS Service starting - available immediately with gTTS")
    logger.info("XTTS v2 will load in background")
    thread = Thread(target=load_xtts_model, daemon=True)
    thread.start()

class TTSRequest(BaseModel):
    text: str
    language: str = "en"

@app.post("/synthesize")
async def synthesize(req: TTSRequest):
    """
    Convert text to speech using XTTS v2 or gTTS fallback.
    Returns WAV audio data.
    """
    global tts_model
    
    try:
        if not req.text or not req.text.strip():
            raise HTTPException(status_code=400, detail="Text cannot be empty")
        
        logger.info(f"Synthesizing text in {req.language}: {req.text[:100]}...")
        
        # Create temporary output file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as output_file:
            output_path = output_file.name
        
        # Try XTTS v2 first, fallback to gTTS
        use_gtts = tts_model is None
        
        if not use_gtts:
            try:
                logger.info("Using XTTS v2 for synthesis")
                tts_model.tts_to_file(
                    text=req.text,
                    file_path=output_path,
                    language=req.language,
                    speaker="Claribel Dervla"
                )
            except Exception as e:
                logger.warning(f"XTTS v2 failed: {e}, falling back to gTTS")
                use_gtts = True
        
        # Use gTTS fallback
        if use_gtts:
            logger.info("Using gTTS for synthesis")
            tts = gTTS(text=req.text, lang=req.language, slow=False)
            # gTTS saves as MP3, but we'll convert it
            mp3_path = output_path.replace('.wav', '.mp3')
            tts.save(mp3_path)
            
            # Convert MP3 to WAV using ffmpeg if available, otherwise return MP3
            try:
                import subprocess
                subprocess.run(['ffmpeg', '-i', mp3_path, '-ar', '16000', '-ac', '1', output_path], 
                             capture_output=True, check=True)
                os.unlink(mp3_path)
            except:
                # If ffmpeg not available, just rename MP3 to use
                os.rename(mp3_path, output_path)
        
        # Read generated audio
        with open(output_path, "rb") as f:
            audio_data = f.read()
        
        # Cleanup
        os.unlink(output_path)
        
        logger.info("Synthesis complete")
        
        return Response(
            content=audio_data,
            media_type="audio/wav",
            headers={"Content-Disposition": "inline"}
        )
    
    except Exception as e:
        logger.error(f"TTS error: {e}")
        raise HTTPException(status_code=500, detail=f"TTS failed: {str(e)}")

@app.post("/synthesize_with_voice")
async def synthesize_with_voice(
    text: str = Form(...),
    language: str = Form("en"),
    reference_audio: UploadFile = File(...)
):
    """
    Convert text to speech with voice cloning from reference audio.
    Uses Coqui XTTS v2 to clone the speaker's voice characteristics.
    
    Returns WAV audio data.
    """
    try:
        if not text or not text.strip():
            raise HTTPException(status_code=400, detail="Text cannot be empty")
        
        if tts_model is None:
            if model_loading:
                raise HTTPException(
                    status_code=503, 
                    detail="XTTS v2 model is still loading. Voice cloning will be available soon. Please try again in a few minutes or use /synthesize endpoint for basic TTS with gTTS."
                )
            else:
                raise HTTPException(
                    status_code=503, 
                    detail="XTTS v2 model failed to load. Voice cloning is not available. Using /synthesize endpoint for basic TTS with gTTS instead."
                )
        
        logger.info(f"Voice cloning synthesis in {language}: {text[:100]}...")
        logger.info(f"Reference audio: {reference_audio.filename}")

        # Save reference audio to temporary file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as ref_file:
            ref_file.write(await reference_audio.read())
            ref_audio_path = ref_file.name

        # Split text into chunks to avoid token limit
        chunks = chunk_text(text)
        logger.info(f"Split text into {len(chunks)} chunk(s) for processing")

        # Generate audio for each chunk
        audio_segments = []
        temp_files = []

        try:
            for i, chunk in enumerate(chunks):
                logger.info(f"Processing chunk {i+1}/{len(chunks)}: {chunk[:50]}...")

                # Create temporary output file for this chunk
                with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as chunk_file:
                    chunk_path = chunk_file.name
                    temp_files.append(chunk_path)

                # Generate speech with voice cloning for this chunk
                tts_model.tts_to_file(
                    text=chunk,
                    file_path=chunk_path,
                    speaker_wav=ref_audio_path,
                    language=language
                )

                # Load audio segment
                audio_segments.append(AudioSegment.from_wav(chunk_path))

            # Combine all audio segments
            logger.info(f"Combining {len(audio_segments)} audio segments...")
            combined_audio = audio_segments[0]
            for segment in audio_segments[1:]:
                combined_audio += segment

            # Export combined audio to temporary file
            with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as output_file:
                output_path = output_file.name

            combined_audio.export(output_path, format="wav")

            # Read generated audio
            with open(output_path, "rb") as f:
                audio_data = f.read()

            logger.info(f"Voice cloning complete: {len(audio_data)} bytes")

        finally:
            # Cleanup all temporary files
            os.unlink(ref_audio_path)
            if 'output_path' in locals():
                os.unlink(output_path)
            for temp_file in temp_files:
                if os.path.exists(temp_file):
                    os.unlink(temp_file)
        
        return Response(
            content=audio_data,
            media_type="audio/wav",
            headers={"Content-Disposition": "inline"}
        )
    
    except Exception as e:
        logger.error(f"Voice cloning error: {e}")
        raise HTTPException(status_code=500, detail=f"Voice cloning failed: {str(e)}")

@app.get("/health")
async def health():
    """Health check endpoint"""
    status = "loading" if model_loading else "ready"
    return {
        "status": status,
        "service": "tts",
        "model": "xtts_v2" if tts_model is not None else "gtts",
        "xtts_loaded": tts_model is not None,
        "fallback_available": True
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=8005)
