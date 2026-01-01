from fastapi import FastAPI, WebSocket, WebSocketDisconnect, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
import torch
import numpy as np
from faster_whisper import WhisperModel
import whisper  # OpenAI Whisper for actual transcription
import asyncio
import json
import os
from collections import deque
import time
from concurrent.futures import ThreadPoolExecutor
from typing import Dict, Optional
from pyannote.audio import Pipeline, Inference
import io
import wave

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

# Load speaker diarization pipeline
print("Loading speaker diarization pipeline...")
try:
    # Don't pass any authentication parameter - will use HF_TOKEN env var if set
    diarization_pipeline = Pipeline.from_pretrained(
        "pyannote/speaker-diarization-3.1",
        use_auth_token=os.getenv("HF_TOKEN")
    )

    # Move to GPU if available
    if DEVICE == "cuda":
        diarization_pipeline.to(torch.device("cuda"))
    
    # Tune parameters for better accuracy
    # min_duration_on: minimum duration of speech to consider it a speaker turn (reduce false positives)
    # min_duration_off: minimum silence between speakers (prevents rapid switching)
    diarization_pipeline.instantiate({
        "segmentation": {
            "min_duration_on": 0.5,  # Minimum 0.5s of speech per segment
            "min_duration_off": 0.3  # Minimum 0.3s silence between speakers
        },
        "clustering": {
            "threshold": 0.7  # Higher = more conservative (fewer but more accurate speaker switches)
        }
    })
    
    print("Speaker diarization pipeline loaded successfully with tuned parameters")
    DIARIZATION_ENABLED = True
except Exception as e:
    print(f"Warning: Could not load speaker diarization pipeline: {e}")
    print("Speaker diarization will be disabled")
    diarization_pipeline = None
    DIARIZATION_ENABLED = False

# Load speaker embedding model for persistent speaker tracking
print("Loading speaker embedding model...")
try:
    embedding_inference = Inference(
        "pyannote/embedding",
        use_auth_token=os.getenv("HF_TOKEN"),
        window="whole",
        device=DEVICE
    )
    EMBEDDING_ENABLED = True
    print("Speaker embedding model loaded successfully")
except Exception as e:
    print(f"Warning: Could not load speaker embedding model: {e}")
    embedding_inference = None
    EMBEDDING_ENABLED = False

# Configuration
SAMPLE_RATE = 16000
CHUNK_DURATION = 1.0  # Process 1 second at a time for streaming
CHUNK_SIZE = int(SAMPLE_RATE * CHUNK_DURATION)
VAD_THRESHOLD = 0.3  # Voice activity detection threshold (lowered for better sensitivity)
SILENCE_DURATION = 1.2  # Seconds of silence to finalize segment
MAX_SEGMENT_DURATION = 10.0  # Max seconds before forcing finalization (new!)

# Speaker tracking configuration
SPEAKER_SIM_THRESHOLD = 0.82
MIN_EMBED_DURATION = 0.8

# Thread pool for CPU-bound operations
executor = ThreadPoolExecutor(max_workers=4)

# Session storage for final high-quality transcriptions
session_transcriptions: Dict[str, dict] = {}
session_speaker_profiles: Dict[str, dict] = {}

def audio_array_to_wav_bytes(audio_array: np.ndarray, sample_rate: int = 16000) -> bytes:
    """Convert numpy audio array to WAV bytes for diarization pipeline"""
    wav_buffer = io.BytesIO()
    with wave.open(wav_buffer, 'wb') as wav_file:
        wav_file.setnchannels(1)  # Mono
        wav_file.setsampwidth(2)  # 16-bit
        wav_file.setframerate(sample_rate)
        # Convert float32 to int16
        audio_int16 = (audio_array * 32767).astype(np.int16)
        wav_file.writeframes(audio_int16.tobytes())
    wav_buffer.seek(0)
    return wav_buffer.read()

def perform_speaker_diarization(audio_array: np.ndarray, min_speakers: Optional[int] = None, max_speakers: Optional[int] = None) -> list:
    """
    Perform speaker diarization on audio
    Returns list of (start_time, end_time, speaker_label) tuples
    """
    if not DIARIZATION_ENABLED or diarization_pipeline is None:
        return []

def _cosine_similarity(a: np.ndarray, b: np.ndarray) -> float:
    denom = (np.linalg.norm(a) * np.linalg.norm(b))
    if denom == 0:
        return 0.0
    return float(np.dot(a, b) / denom)

def _get_session_state(session_id: str) -> dict:
    state = session_speaker_profiles.get(session_id)
    if state is None:
        state = {"next_id": 0, "profiles": []}
        session_speaker_profiles[session_id] = state
    return state

def _compute_embedding(audio_array: np.ndarray, start: float, end: float) -> Optional[np.ndarray]:
    if not EMBEDDING_ENABLED or embedding_inference is None:
        return None
    start_idx = max(0, int(start * SAMPLE_RATE))
    end_idx = min(len(audio_array), int(end * SAMPLE_RATE))
    if end_idx <= start_idx:
        return None
    segment = audio_array[start_idx:end_idx]
    if len(segment) < int(MIN_EMBED_DURATION * SAMPLE_RATE):
        return None
    waveform = torch.from_numpy(segment).float().unsqueeze(0)
    emb = embedding_inference({"waveform": waveform, "sample_rate": SAMPLE_RATE})
    if isinstance(emb, torch.Tensor):
        emb = emb.detach().cpu().numpy()
    return np.squeeze(emb)

def assign_persistent_speakers(speaker_segments: list, audio_array: np.ndarray, session_id: Optional[str]) -> list:
    if not session_id or not speaker_segments or not EMBEDDING_ENABLED:
        return speaker_segments

    state = _get_session_state(session_id)
    label_cache: Dict[str, str] = {}

    for seg in speaker_segments:
        label = seg.get("speaker", "SPEAKER_00")
        if label in label_cache:
            seg["speaker"] = label_cache[label]
            continue

        emb = _compute_embedding(audio_array, seg["start"], seg["end"])
        if emb is None:
            continue

        best_id = None
        best_sim = -1.0
        for profile in state["profiles"]:
            sim = _cosine_similarity(emb, profile["embedding"])
            if sim > best_sim:
                best_sim = sim
                best_id = profile["id"]

        if best_id is not None and best_sim >= SPEAKER_SIM_THRESHOLD:
            for profile in state["profiles"]:
                if profile["id"] == best_id:
                    count = profile["count"]
                    profile["embedding"] = (profile["embedding"] * count + emb) / (count + 1)
                    profile["count"] = count + 1
                    break
            seg["speaker"] = best_id
            label_cache[label] = best_id
        else:
            new_id = f"SPEAKER_{state['next_id']:02d}"
            state["next_id"] += 1
            state["profiles"].append({
                "id": new_id,
                "embedding": emb,
                "count": 1
            })
            seg["speaker"] = new_id
            label_cache[label] = new_id

    return speaker_segments

    try:
        print("🎭 Starting speaker diarization...")

        # Convert audio to WAV format in memory
        wav_bytes = audio_array_to_wav_bytes(audio_array, SAMPLE_RATE)
        wav_io = io.BytesIO(wav_bytes)

        # Run diarization
        diarization_kwargs = {"uri": "stream", "audio": wav_io}
        if min_speakers is not None:
            diarization_kwargs["min_speakers"] = min_speakers
        if max_speakers is not None:
            diarization_kwargs["max_speakers"] = max_speakers
        diarization = diarization_pipeline(diarization_kwargs)

        # Extract speaker segments (filter out very short segments)
        MIN_SEGMENT_DURATION = 0.4  # Ignore segments shorter than 0.4 seconds
        speaker_segments = []
        for turn, _, speaker in diarization.itertracks(yield_label=True):
            duration = turn.end - turn.start
            if duration >= MIN_SEGMENT_DURATION:
                speaker_segments.append({
                    "start": turn.start,
                    "end": turn.end,
                    "speaker": speaker
                })
            else:
                print(f"⚠️ Filtering out short segment: {duration:.2f}s from {speaker}")

        print(f"✅ Diarization complete: found {len(set(s['speaker'] for s in speaker_segments))} speakers")
        
        # Post-process: smooth rapid speaker changes (reduces confusion)
        speaker_segments = smooth_speaker_changes(speaker_segments)
        
        return speaker_segments

    except Exception as e:
        print(f"❌ Speaker diarization failed: {e}")
        import traceback
        traceback.print_exc()
        return []

def smooth_speaker_changes(speaker_segments: list, min_switch_duration: float = 0.5) -> list:
    """
    Smooth out rapid speaker changes that are likely errors.
    If a speaker appears for less than min_switch_duration between two segments of the same speaker,
    merge it with the surrounding speaker.
    """
    if len(speaker_segments) < 3:
        return speaker_segments
    
    smoothed = [speaker_segments[0]]
    
    for i in range(1, len(speaker_segments) - 1):
        current = speaker_segments[i]
        prev = smoothed[-1]
        next_seg = speaker_segments[i + 1]
        
        current_duration = current["end"] - current["start"]
        
        # If current segment is very short AND surrounded by same speaker, skip it
        if (current_duration < min_switch_duration and 
            prev["speaker"] == next_seg["speaker"] and 
            current["speaker"] != prev["speaker"]):
            print(f"🔧 Smoothing: merging short {current['speaker']} segment ({current_duration:.2f}s) into {prev['speaker']}")
            # Extend previous segment to cover the gap
            smoothed[-1]["end"] = current["end"]
        else:
            smoothed.append(current)
    
    # Add last segment
    smoothed.append(speaker_segments[-1])
    
    return smoothed

def assign_speakers_to_segments(transcription_segments: list, speaker_segments: list) -> list:
    """
    Assign speaker labels to transcription segments based on temporal overlap
    """
    if not speaker_segments:
        # No diarization available, return segments without speaker info
        return transcription_segments

    result = []
    for seg in transcription_segments:
        seg_start = seg["start"]
        seg_end = seg["end"]
        seg_mid = (seg_start + seg_end) / 2

        # Find the speaker segment that overlaps most with this transcription segment
        best_speaker = None
        max_overlap = 0

        for spk in speaker_segments:
            # Calculate overlap
            overlap_start = max(seg_start, spk["start"])
            overlap_end = min(seg_end, spk["end"])
            overlap = max(0, overlap_end - overlap_start)

            if overlap > max_overlap:
                max_overlap = overlap
                best_speaker = spk["speaker"]

        # If no overlap, use midpoint to find closest speaker segment
        if best_speaker is None:
            for spk in speaker_segments:
                if spk["start"] <= seg_mid <= spk["end"]:
                    best_speaker = spk["speaker"]
                    break

        # Still no speaker? Use the closest one
        if best_speaker is None and speaker_segments:
            closest_spk = min(speaker_segments,
                            key=lambda s: min(abs(s["start"] - seg_mid), abs(s["end"] - seg_mid)))
            best_speaker = closest_spk["speaker"]

        result.append({
            **seg,
            "speaker": best_speaker or "SPEAKER_00"
        })

    return result

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
                # Step 1: Transcribe with Whisper
                condition_on_prev = transcriber.language is not None
                result = whisper_model.transcribe(
                    full_audio,
                    language=transcriber.language,
                    fp16=(DEVICE == "cuda"),
                    verbose=False,
                    temperature=0.0,
                    compression_ratio_threshold=2.4,
                    condition_on_previous_text=condition_on_prev,
                    word_timestamps=True  # Get word-level timestamps for better segmentation
                )

                final_text = result["text"].strip()
                segments_with_timestamps = result.get("segments", [])

                print(f"✅ High-quality transcription complete: '{final_text[:100]}...'")
                print(f"   Got {len(segments_with_timestamps)} segments with timestamps")

                # Step 2: Perform speaker diarization
                speaker_segments = perform_speaker_diarization(full_audio)

                # Step 3: Merge transcription segments with speaker labels
                transcription_segments = [
                    {
                        "text": seg["text"].strip(),
                        "start": seg["start"],
                        "end": seg["end"]
                    }
                    for seg in segments_with_timestamps if seg["text"].strip()
                ]

                # Assign speakers to segments
                segments_with_speakers = assign_speakers_to_segments(
                    transcription_segments,
                    speaker_segments
                )

                # Count unique speakers
                unique_speakers = len(set(s.get("speaker", "SPEAKER_00") for s in segments_with_speakers))
                print(f"👥 Identified {unique_speakers} unique speaker(s)")

                # Store the high-quality result with speaker labels
                session_transcriptions[session_id] = {
                    "full_text": final_text,
                    "segments": segments_with_speakers,
                    "language": result.get("language", "unknown"),
                    "duration": len(full_audio) / SAMPLE_RATE,
                    "num_speakers": unique_speakers
                }

                print(f"💾 Stored high-quality transcription with speaker diarization for session {session_id}")

            except Exception as e:
                print(f"❌ High-quality transcription failed: {e}")
                import traceback
                traceback.print_exc()
        
    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
        process_task.cancel()

@app.post("/transcribe")
async def transcribe_audio(request: Request):
    """HTTP endpoint for batch audio transcription"""
    try:
        # Get audio data from request body
        audio_data = await request.body()

        # Get language from header (optional)
        language = request.headers.get("x-language", None)
        if language == "auto" or language == "":
            language = None

        print(f"📝 Transcription request: {len(audio_data)} bytes, language={language}")

        # Convert WAV bytes to numpy array
        import io
        import wave

        wav_file = io.BytesIO(audio_data)
        with wave.open(wav_file, 'rb') as wav:
            frames = wav.readframes(wav.getnframes())
            audio_array = np.frombuffer(frames, dtype=np.int16).astype(np.float32) / 32768.0

        print(f"   Audio: {len(audio_array)} samples ({len(audio_array)/SAMPLE_RATE:.1f}s)")

        # Transcribe with Whisper
        result = whisper_model.transcribe(
            audio_array,
            language=language,
            fp16=(DEVICE == "cuda"),
            verbose=False,
            temperature=0.0,
            compression_ratio_threshold=2.4,
            condition_on_previous_text=True
        )

        text = result["text"].strip()
        detected_lang = result.get("language", "unknown")

        print(f"   ✅ Transcribed: '{text[:100]}...' (lang: {detected_lang})")

        return JSONResponse(content={"text": text, "language": detected_lang})

    except Exception as e:
        print(f"❌ Transcription error: {e}")
        import traceback
        traceback.print_exc()
        return JSONResponse(
            status_code=500,
            content={"error": str(e)}
        )

@app.post("/detect-language")
async def detect_language(request: Request):
    """HTTP endpoint for language detection"""
    try:
        # Get audio data from request body
        audio_data = await request.body()

        print(f"🔍 Language detection request: {len(audio_data)} bytes")

        # Convert WAV bytes to numpy array
        import io
        import wave

        wav_file = io.BytesIO(audio_data)
        with wave.open(wav_file, 'rb') as wav:
            frames = wav.readframes(wav.getnframes())
            audio_array = np.frombuffer(frames, dtype=np.int16).astype(np.float32) / 32768.0

        # Transcribe just to detect language (fast, no full transcription)
        result = whisper_model.transcribe(
            audio_array[:SAMPLE_RATE * 30],  # Use first 30 seconds max
            fp16=(DEVICE == "cuda"),
            verbose=False,
            temperature=0.0
        )

        detected_lang = result.get("language", "unknown")
        text_sample = result["text"].strip()[:100]

        print(f"   ✅ Detected language: {detected_lang}, sample: '{text_sample}...'")

        return JSONResponse(content={
            "language": detected_lang,
            "text": text_sample
        })

    except Exception as e:
        print(f"❌ Language detection error: {e}")
        import traceback
        traceback.print_exc()
        return JSONResponse(
            status_code=500,
            content={"error": str(e)}
        )

@app.post("/transcribe-with-diarization")
async def transcribe_with_diarization(request: Request):
    """HTTP endpoint for batch audio transcription with speaker diarization"""
    try:
        # Get audio data from request body
        audio_data = await request.body()

        # Get language from header (optional)
        language = request.headers.get("x-language", None)
        if language == "auto" or language == "":
            language = None

        session_id = request.query_params.get("session_id")
        print(f"📝🎭 Transcription + Diarization request: {len(audio_data)} bytes, language={language}, session={session_id}")

        # Convert WAV bytes to numpy array
        import io
        import wave

        wav_file = io.BytesIO(audio_data)
        with wave.open(wav_file, 'rb') as wav:
            frames = wav.readframes(wav.getnframes())
            audio_array = np.frombuffer(frames, dtype=np.int16).astype(np.float32) / 32768.0

        print(f"   Audio: {len(audio_array)} samples ({len(audio_array)/SAMPLE_RATE:.1f}s)")

        # Step 1: Transcribe with Whisper
        condition_on_prev = language is not None
        result = whisper_model.transcribe(
            audio_array,
            language=language,
            fp16=(DEVICE == "cuda"),
            verbose=False,
            temperature=0.0,
            compression_ratio_threshold=2.4,
            condition_on_previous_text=condition_on_prev,
            word_timestamps=True
        )

        full_text = result["text"].strip()
        detected_lang = result.get("language", "unknown")
        segments_with_timestamps = result.get("segments", [])

        print(f"   ✅ Transcribed: '{full_text[:100]}...' (lang: {detected_lang})")
        print(f"   Got {len(segments_with_timestamps)} segments")

        # Step 2: Perform speaker diarization
        min_speakers = request.query_params.get("min_speakers")
        max_speakers = request.query_params.get("max_speakers")
        min_speakers = int(min_speakers) if min_speakers and min_speakers.isdigit() else None
        max_speakers = int(max_speakers) if max_speakers and max_speakers.isdigit() else None

        speaker_segments = perform_speaker_diarization(audio_array, min_speakers=min_speakers, max_speakers=max_speakers)
        # Step 2.5: Stabilize speaker IDs across chunks (if session_id provided)
        speaker_segments = assign_persistent_speakers(speaker_segments, audio_array, session_id)

        # Step 3: Merge transcription segments with speaker labels
        transcription_segments = [
            {
                "text": seg["text"].strip(),
                "start": seg["start"],
                "end": seg["end"]
            }
            for seg in segments_with_timestamps if seg["text"].strip()
        ]

        # Assign speakers to segments
        segments_with_speakers = assign_speakers_to_segments(
            transcription_segments,
            speaker_segments
        )

        # Count unique speakers
        unique_speakers = len(set(s.get("speaker", "SPEAKER_00") for s in segments_with_speakers))
        print(f"   👥 Identified {unique_speakers} unique speaker(s)")

        return JSONResponse(content={
            "text": full_text,
            "language": detected_lang,
            "segments": segments_with_speakers,
            "num_speakers": unique_speakers
        })

    except Exception as e:
        print(f"❌ Transcription + Diarization error: {e}")
        import traceback
        traceback.print_exc()
        return JSONResponse(
            status_code=500,
            content={"error": str(e)}
        )

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
    # Configure with longer timeouts for audio processing
    uvicorn.run(
        app, 
        host="0.0.0.0", 
        port=8003,
        timeout_keep_alive=300,  # 5 minutes keep-alive
        timeout_graceful_shutdown=30,  # 30 seconds for graceful shutdown
        ws_ping_interval=60,  # Send WebSocket pings every 60 seconds
        ws_ping_timeout=120  # Wait up to 2 minutes for pong response
    )
