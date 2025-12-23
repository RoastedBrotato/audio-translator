#!/usr/bin/env python3
"""
TTS Service with Coqui XTTS v2 Voice Cloning
Provides high-quality voice cloning from reference audio.
"""
from fastapi import FastAPI, File, UploadFile, Form, HTTPException
from fastapi.responses import Response
from pydantic import BaseModel
from TTS.api import TTS
import tempfile
import os
import logging

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = FastAPI(title="TTS Service with XTTS v2")

# Global TTS model
tts_model = None

@app.on_event("startup")
async def load_model():
    """Load XTTS v2 model on startup"""
    global tts_model
    try:
        logger.info("Loading XTTS v2 model... This may take a minute...")
        tts_model = TTS("tts_models/multilingual/multi-dataset/xtts_v2")
        logger.info("✓ XTTS v2 model loaded successfully!")
    except Exception as e:
        logger.error(f"Failed to load XTTS v2: {e}")
        raise

class TTSRequest(BaseModel):
    text: str
    language: str = "en"

@app.post("/synthesize")
async def synthesize(req: TTSRequest):
    """
    Convert text to speech using XTTS v2.
    Note: XTTS v2 is primarily for voice cloning. Use /synthesize_with_voice for best results.
    Returns WAV audio data.
    """
    try:
        if not req.text or not req.text.strip():
            raise HTTPException(status_code=400, detail="Text cannot be empty")
        
        if tts_model is None:
            raise HTTPException(status_code=503, detail="TTS model not loaded")
        
        logger.info(f"Synthesizing text in {req.language}: {req.text[:100]}...")
        logger.warning("Using /synthesize without voice cloning. For better results, use /synthesize_with_voice")
        
        # XTTS v2 requires speaker_wav for voice cloning
        # Since no reference audio provided, we'll compute speaker latents from the text itself
        # This provides a generic voice
        
        # Create temporary output file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as output_file:
            output_path = output_file.name
        
        # Generate speech using text conditioning (generic voice)
        # For XTTS v2, we use the tts_to_file with just text and language
        # The model will use its default speaker embedding
        tts_model.tts_to_file(
            text=req.text,
            file_path=output_path,
            language=req.language,
            speaker="Claribel Dervla"  # Use a default speaker from XTTS v2
        )
        
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
            raise HTTPException(status_code=503, detail="TTS model not loaded")
        
        logger.info(f"Voice cloning synthesis in {language}: {text[:100]}...")
        logger.info(f"Reference audio: {reference_audio.filename}")
        
        # Save reference audio to temporary file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as ref_file:
            ref_file.write(await reference_audio.read())
            ref_audio_path = ref_file.name
        
        # Create temporary output file
        with tempfile.NamedTemporaryFile(delete=False, suffix=".wav") as output_file:
            output_path = output_file.name
        
        # Generate speech with voice cloning
        logger.info("Cloning voice and generating speech...")
        tts_model.tts_to_file(
            text=text,
            file_path=output_path,
            speaker_wav=ref_audio_path,
            language=language
        )
        
        # Read generated audio
        with open(output_path, "rb") as f:
            audio_data = f.read()
        
        # Cleanup
        os.unlink(ref_audio_path)
        os.unlink(output_path)
        
        logger.info(f"Voice cloning complete: {len(audio_data)} bytes")
        
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
    return {
        "status": "ok",
        "service": "tts",
        "model": "xtts_v2",
        "voice_cloning": tts_model is not None
    }

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="127.0.0.1", port=8005)
