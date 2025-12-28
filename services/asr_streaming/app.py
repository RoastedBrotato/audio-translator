from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
import torch
import numpy as np
from faster_whisper import WhisperModel
import whisper  # OpenAI Whisper for actual transcription
import asyncio
import json
from collections import deque
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Optional

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Load OpenAI Whisper model (simpler, no generator issues)
MODEL_SIZE = "medium"  # Medium model - best balance for 8GB GPU
DEVICE = "cuda" if torch.cuda.is_available() else "cpu"

print(f"Loading OpenAI Whisper model: {MODEL_SIZE} on {DEVICE}")
whisper_model = whisper.load_model(MODEL_SIZE, device=DEVICE)
print(f"OpenAI Whisper model loaded successfully")

# Load Silero VAD
print("Loading Silero VAD...")
vad_model, utils = torch.hub.load(repo_or_dir='snakers4/silero-vad',
                                   model='silero_vad',
                                   force_reload=False)
(get_speech_timestamps, _, read_audio, _, _) = utils
print("VAD loaded successfully")

# Configuration
SAMPLE_RATE = 16000
CHUNK_DURATION = 1.0  # Process 1 second at a time for streaming
CHUNK_SIZE = int(SAMPLE_RATE * CHUNK_DURATION)
VAD_THRESHOLD = 0.3  # Voice activity detection threshold (lowered for better sensitivity)
SILENCE_DURATION = 1.2  # Seconds of silence to finalize segment
MAX_SEGMENT_DURATION = 10.0  # Max seconds before forcing finalization (new!)

# Thread pool for CPU-bound operations
executor = ThreadPoolExecutor(max_workers=4)

# Session storage for final high-quality transcriptions
session_transcriptions: Dict[str, dict] = {}

def transcribe_audio_sync(audio_array: np.ndarray, language: str = None) -> str:
    """Synchronous transcription function to run in thread pool"""
    print(f"[THREAD] Starting transcription of {len(audio_array)} samples, language={language}")
    try:
        print("[THREAD] Calling model.transcribe()...")
        segments, info = model.transcribe(
            audio_array,
            language=language,
            beam_size=1,  # Faster for streaming
            best_of=1,
            vad_filter=False,  # We're doing VAD manually
            condition_on_previous_text=True
        )

        print(f"[THREAD] Transcription returned, detected language: {info.language}")

        # Process segments immediately - do NOT convert to list as it hangs
        print("[THREAD] Processing segments directly from generator...")
        text_parts = []
        segment_count = 0

        try:
            for seg in segments:
                segment_count += 1
                segment_text = seg.text.strip()
                print(f"[THREAD] Segment {segment_count}: '{segment_text}'")
                if segment_text:
                    text_parts.append(segment_text)
                # Break after 50 segments to avoid hanging
                if segment_count > 50:
                    print("[THREAD] Reached max segment limit")
                    break
        except Exception as e:
            print(f"[THREAD] Error iterating segments: {e}")
            import traceback
            traceback.print_exc()

        print(f"[THREAD] Processed {segment_count} segments total")
        text = " ".join(text_parts)
        print(f"[THREAD] Final text: '{text}'")
        return text
    except Exception as e:
        print(f"[THREAD] Transcription error in sync function: {e}")
        import traceback
        traceback.print_exc()
        return ""

class StreamingTranscriber:
    def __init__(self, language: str = None):
        self.language = language
        self.audio_buffer = deque(maxlen=int(SAMPLE_RATE * 30))  # 30 second rolling buffer
        self.segment_buffer = []
        self.last_speech_time = time.time()
        self.segment_start_time = time.time()  # Track when segment started
        self.current_text = ""
        self.finalized_segments = []
        
    def add_audio(self, audio_chunk: np.ndarray):
        """Add audio chunk to buffer"""
        self.audio_buffer.extend(audio_chunk)
        self.segment_buffer.extend(audio_chunk)
        
    def check_vad(self, audio_chunk: np.ndarray) -> float:
        """Check if audio contains speech"""
        if len(audio_chunk) < 512:
            return 0.0

        try:
            # Ensure audio is float32 and normalized
            audio_chunk = audio_chunk.astype(np.float32)

            # VAD expects exactly 512 samples for 16kHz
            # Take the last 512 samples if we have more
            if len(audio_chunk) > 512:
                audio_chunk = audio_chunk[-512:]

            # Convert to tensor
            audio_tensor = torch.from_numpy(audio_chunk).float()

            # Get speech probability
            speech_prob = vad_model(audio_tensor, SAMPLE_RATE).item()
            return speech_prob
        except Exception as e:
            print(f"VAD error: {e}")
            # Calculate RMS as fallback
            rms = np.sqrt(np.mean(audio_chunk ** 2))
            # If RMS > 0.01, likely speech
            return 0.9 if rms > 0.01 else 0.1
    
    async def process_streaming(self, websocket: WebSocket):
        """Process audio in streaming mode"""
        last_transcribe_time = time.time()
        min_transcribe_interval = 2.0  # Wait longer between transcriptions for better quality
        min_audio_length = SAMPLE_RATE * 3  # Minimum 3 seconds of audio before transcribing

        print(f"Starting processing loop for session")

        while True:
            await asyncio.sleep(0.1)  # Check every 100ms

            current_time = time.time()

            # Need minimum audio before processing
            if len(self.segment_buffer) < CHUNK_SIZE:
                continue

            # Get the latest chunk for VAD
            audio_chunk = np.array(list(self.segment_buffer)[-CHUNK_SIZE:])

            try:
                speech_prob = self.check_vad(audio_chunk)
                # Only log if speech detected
                if speech_prob > VAD_THRESHOLD:
                    print(f"Speech detected: prob={speech_prob:.3f}, buffer_size={len(self.segment_buffer)}")
            except Exception as e:
                print(f"VAD error: {e}, using fallback")
                # Calculate RMS as fallback
                rms = np.sqrt(np.mean(audio_chunk ** 2))
                speech_prob = 0.9 if rms > 0.01 else 0.1

            if speech_prob > VAD_THRESHOLD:
                self.last_speech_time = current_time

                # Transcribe if:
                # 1. Enough time has passed since last transcription
                # 2. We have enough audio (at least 2 seconds)
                time_since_last = current_time - last_transcribe_time
                has_enough_audio = len(self.segment_buffer) >= min_audio_length

                if time_since_last >= min_transcribe_interval and has_enough_audio:
                    # Transcribe the ENTIRE segment buffer
                    audio_array = np.array(self.segment_buffer, dtype=np.float32)

                    print(f"Transcribing {len(audio_array)} samples ({len(audio_array)/SAMPLE_RATE:.1f}s)...")

                    try:
                        # Use OpenAI Whisper
                        print("Transcribing with OpenAI Whisper...")

                        # Transcribe using OpenAI Whisper with better settings
                        result = whisper_model.transcribe(
                            audio_array,
                            language=self.language,
                            fp16=(DEVICE == "cuda"),
                            verbose=False,
                            temperature=0.0,  # More deterministic
                            compression_ratio_threshold=2.4,
                            condition_on_previous_text=True
                        )

                        text = result["text"].strip()
                        detected_lang = result.get("language", "unknown")

                        print(f"Transcription complete! Detected: {detected_lang}")
                        print(f"Result: '{text}'")

                        # Filter out hallucinations (repetitive punctuation, thank you, etc.)
                        is_hallucination = False
                        if text:
                            # Check for repetitive characters (hallucination indicator)
                            if len(set(text.replace(" ", ""))) <= 3:  # Only 3 or fewer unique chars
                                is_hallucination = True
                                print(f"⚠️ Hallucination detected (repetitive): '{text[:50]}...'")
                            # Check for common hallucination phrases
                            elif any(phrase in text.lower() for phrase in ["thank you", "thanks for watching", "subscribe"]):
                                is_hallucination = True
                                print(f"⚠️ Hallucination detected (common phrase): '{text}'")

                        if text and not is_hallucination:
                            # Update current text (don't check if different - Whisper refines as it gets more audio)
                            self.current_text = text

                            # Send partial result
                            await websocket.send_json({
                                "type": "partial",
                                "text": text,
                                "is_final": False
                            })

                            print(f"✅ Sent partial: '{text}'")
                            last_transcribe_time = current_time
                        elif not text:
                            print("No text transcribed (empty result)")
                        elif is_hallucination:
                            print("Skipping hallucinated text")

                    except Exception as e:
                        print(f"Transcription error: {e}")
                        import traceback
                        traceback.print_exc()

            # Check if we should finalize the segment
            silence_duration = current_time - self.last_speech_time
            segment_duration = current_time - self.segment_start_time

            # Finalize if: 1) silence detected OR 2) segment too long
            should_finalize = (silence_duration >= SILENCE_DURATION and self.current_text) or \
                             (segment_duration >= MAX_SEGMENT_DURATION and self.current_text)

            if should_finalize:
                reason = "silence" if silence_duration >= SILENCE_DURATION else f"max duration ({segment_duration:.1f}s)"
                print(f"Finalizing segment after {reason}: '{self.current_text}'")

                await websocket.send_json({
                    "type": "final",
                    "text": self.current_text,
                    "is_final": True
                })

                self.finalized_segments.append(self.current_text)
                self.current_text = ""
                self.segment_buffer.clear()
                self.segment_start_time = current_time  # Reset timer
                print("Segment buffer cleared, ready for next segment")

@app.websocket("/stream")
async def websocket_endpoint(websocket: WebSocket):
    await websocket.accept()

    # Get language and session ID from query params
    language = websocket.query_params.get("language", "en")
    session_id = websocket.query_params.get("session_id", "unknown")
    if language == "auto":
        language = None

    print(f"Streaming connection established, session: {session_id}, language: {language}")

    transcriber = StreamingTranscriber(language=language)
    
    # Start processing task
    process_task = asyncio.create_task(transcriber.process_streaming(websocket))
    
    chunk_count = 0
    try:
        while True:
            # Receive audio data
            data = await websocket.receive_bytes()

            if len(data) == 0:
                continue

            chunk_count += 1

            # Convert bytes to float32 PCM
            audio_chunk = np.frombuffer(data, dtype=np.int16).astype(np.float32) / 32768.0

            # Add to transcriber
            transcriber.add_audio(audio_chunk)

            # Log every 50 chunks for debugging
            if chunk_count % 50 == 0:
                print(f"Received chunk #{chunk_count}: {len(data)} bytes, {len(audio_chunk)} samples, buffer total: {len(transcriber.audio_buffer)} samples")

            # Log occasionally
            if len(transcriber.audio_buffer) % 160000 == 0:  # Every 10 seconds
                print(f"Received audio: total {len(transcriber.audio_buffer)} samples")
            
    except WebSocketDisconnect:
        print("Client disconnected")
        process_task.cancel()

        # Return full recording audio for final processing
        full_audio = np.array(list(transcriber.audio_buffer), dtype=np.float32)

        print(f"Session ended. Collected {len(full_audio)} samples ({len(full_audio)/SAMPLE_RATE:.1f}s)")
        print(f"Finalized {len(transcriber.finalized_segments)} segments")

        # Perform high-quality re-transcription of full audio
        if len(full_audio) > SAMPLE_RATE * 2:  # Only if we have at least 2 seconds
            print("🔄 Starting high-quality re-transcription of full audio...")
            try:
                result = whisper_model.transcribe(
                    full_audio,
                    language=transcriber.language,
                    fp16=(DEVICE == "cuda"),
                    verbose=False,
                    temperature=0.0,
                    compression_ratio_threshold=2.4,
                    condition_on_previous_text=True,
                    word_timestamps=True  # Get word-level timestamps for better segmentation
                )

                final_text = result["text"].strip()
                segments_with_timestamps = result.get("segments", [])

                print(f"✅ High-quality transcription complete: '{final_text[:100]}...'")
                print(f"   Got {len(segments_with_timestamps)} segments with timestamps")

                # Store the high-quality result
                session_transcriptions[session_id] = {
                    "full_text": final_text,
                    "segments": [
                        {
                            "text": seg["text"].strip(),
                            "start": seg["start"],
                            "end": seg["end"]
                        }
                        for seg in segments_with_timestamps if seg["text"].strip()
                    ],
                    "language": result.get("language", "unknown"),
                    "duration": len(full_audio) / SAMPLE_RATE
                }

                print(f"💾 Stored high-quality transcription for session {session_id}")

            except Exception as e:
                print(f"❌ High-quality transcription failed: {e}")
                import traceback
                traceback.print_exc()
        
    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
        process_task.cancel()

@app.get("/health")
async def health():
    return {"status": "ok", "device": DEVICE, "model": MODEL_SIZE}

@app.get("/transcription/{session_id}")
async def get_final_transcription(session_id: str):
    """Get the final high-quality transcription for a session"""
    if session_id in session_transcriptions:
        return JSONResponse(content={
            "success": True,
            "data": session_transcriptions[session_id]
        })
    else:
        return JSONResponse(
            status_code=404,
            content={
                "success": False,
                "error": "Session not found or still processing"
            }
        )

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8003)
