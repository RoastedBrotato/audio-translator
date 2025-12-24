from fastapi import FastAPI, Request, HTTPException
from fastapi.middleware.cors import CORSMiddleware
import whisper
import numpy as np
import wave
import io
import os

app = FastAPI()

# Add CORS middleware to allow requests from the web frontend
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # In production, specify your frontend URL
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Pick a model size:
# - "tiny" = fastest, lower accuracy (~40MB)
# - "base" = still fast, better accuracy (~75MB)
# - "small" = good accuracy but slower (~244MB) ← USING THIS FOR BETTER ACCURACY
MODEL_SIZE = os.getenv("ASR_MODEL", "small")  # Upgraded to "small" for better recognition

# Load the model
model = whisper.load_model(MODEL_SIZE)
print(f"Loaded Whisper model: {MODEL_SIZE}")


def wav_bytes_to_float32_mono(wav_bytes: bytes):
    """Parse WAV (PCM16) bytes -> float32 mono numpy array in [-1, 1], return (audio, sample_rate)."""
    try:
        with wave.open(io.BytesIO(wav_bytes), "rb") as wf:
            sr = wf.getframerate()
            ch = wf.getnchannels()
            sampwidth = wf.getsampwidth()
            nframes = wf.getnframes()
            pcm = wf.readframes(nframes)
    except wave.Error as e:
        raise HTTPException(status_code=400, detail=f"Invalid WAV: {e}")

    if sampwidth != 2:
        raise HTTPException(status_code=400, detail=f"Expected 16-bit PCM WAV, got sampwidth={sampwidth}")
    if ch not in (1, 2):
        raise HTTPException(status_code=400, detail=f"Expected 1 or 2 channels, got {ch}")

    audio = np.frombuffer(pcm, dtype=np.int16).astype(np.float32) / 32768.0
    if ch == 2:
        audio = audio.reshape(-1, 2).mean(axis=1)  # downmix stereo -> mono

    return audio, sr


@app.post("/transcribe")
async def transcribe(req: Request):
    wav_bytes = await req.body()
    audio, sr = wav_bytes_to_float32_mono(wav_bytes)

    # Your pipeline is already producing 16kHz; enforce it for clean results.
    if sr != 16000:
        # For now, fail fast so you notice if something changes upstream.
        # (We can add resampling later if needed.)
        raise HTTPException(status_code=400, detail=f"Expected 16000 Hz WAV, got {sr}")

    # Optional config via headers (you can wire these later from Go)
    # - language: e.g. "en", "ar", or omit for auto
    language = req.headers.get("x-language", "").strip() or None

    # Transcribe using whisper with optimizations for speed
    import time
    start_time = time.time()
    
    # Check if audio is too quiet (RMS check)
    rms = np.sqrt(np.mean(audio ** 2))
    print(f"Audio RMS: {rms:.6f}")
    
    result = model.transcribe(
        audio,
        language=language,
        task="transcribe",
        fp16=False,  # Use FP32 for CPU
        beam_size=1,  # Faster, less accurate
        best_of=1,    # No sampling
        temperature=0,  # Deterministic
        condition_on_previous_text=False,  # Don't use context
        logprob_threshold=-1.0,  # More permissive
        no_speech_threshold=0.4  # Lower threshold (default 0.6) to catch quieter speech
    )

    text = result["text"].strip()
    elapsed = time.time() - start_time
    print(f"Transcribed in {elapsed:.2f}s: '{text}'")  # Debug logging
    return {"text": text}


@app.post("/detect-language")
async def detect_language(req: Request):
    """Detect the language of the audio without transcribing."""
    wav_bytes = await req.body()
    audio, sr = wav_bytes_to_float32_mono(wav_bytes)

    if sr != 16000:
        raise HTTPException(status_code=400, detail=f"Expected 16000 Hz WAV, got {sr}")

    import time
    start_time = time.time()
    
    # Use Whisper's detect_language function for faster detection
    # We'll transcribe a small portion to detect language
    audio_segment = audio[:16000 * 30]  # Use first 30 seconds max
    
    result = model.transcribe(
        audio_segment,
        language=None,  # Let Whisper auto-detect
        task="transcribe",
        fp16=False,
        beam_size=1,
        best_of=1,
        temperature=0,
        condition_on_previous_text=False
    )
    
    detected_language = result.get("language", "en")
    text = result["text"].strip()
    elapsed = time.time() - start_time
    
    print(f"Detected language: {detected_language} in {elapsed:.2f}s")
    
    return {
        "language": detected_language,
        "text": text,
        "segments": result.get("segments", [])
    }

