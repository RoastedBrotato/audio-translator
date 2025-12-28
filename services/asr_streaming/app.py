from fastapi import FastAPI, WebSocket, WebSocketDisconnect
from fastapi.middleware.cors import CORSMiddleware
import torch
import numpy as np
from faster_whisper import WhisperModel
import asyncio
import json
from collections import deque
import time

app = FastAPI()

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

# Load faster-whisper with GPU support
MODEL_SIZE = "medium"  # Upgraded to medium for better Urdu accuracy
DEVICE = "cuda" if torch.cuda.is_available() else "cpu"
COMPUTE_TYPE = "float16" if DEVICE == "cuda" else "int8"

print(f"Loading Whisper model: {MODEL_SIZE} on {DEVICE} with compute_type={COMPUTE_TYPE}")
model = WhisperModel(MODEL_SIZE, device=DEVICE, compute_type=COMPUTE_TYPE)
print(f"Model loaded successfully")

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
VAD_THRESHOLD = 0.5  # Voice activity detection threshold
SILENCE_DURATION = 0.8  # Seconds of silence to finalize segment
MAX_SEGMENT_DURATION = 10.0  # Max seconds before forcing finalization (new!)

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
        min_transcribe_interval = 0.5  # Minimum 500ms between transcriptions
        
        print(f"Starting processing loop for session")
        
        while True:
            await asyncio.sleep(0.1)  # Check every 100ms
            
            if len(self.segment_buffer) < CHUNK_SIZE:
                continue
            
            current_time = time.time()
            
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
                
                # Transcribe if enough time has passed
                if current_time - last_transcribe_time >= min_transcribe_interval:
                    # Transcribe the current segment buffer
                    audio_array = np.array(self.segment_buffer, dtype=np.float32)
                    
                    print(f"Transcribing {len(audio_array)} samples...")
                    
                    try:
                        segments, info = model.transcribe(
                            audio_array,
                            language=self.language,
                            beam_size=1,  # Faster for streaming
                            best_of=1,
                            vad_filter=False,  # We're doing VAD manually
                            condition_on_previous_text=True
                        )
                        
                        # Combine all segments
                        text = " ".join([seg.text.strip() for seg in segments])
                        
                        print(f"Transcription result: '{text}'")
                        
                        if text and text != self.current_text:
                            self.current_text = text
                            
                            # Send partial result
                            await websocket.send_json({
                                "type": "partial",
                                "text": text,
                                "is_final": False
                            })
                            
                            print(f"Sent partial: '{text}'")
                            
                            last_transcribe_time = current_time
                            
                    except Exception as e:
                        print(f"Transcription error: {e}")
            
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

@app.websocket("/stream")
async def websocket_endpoint(websocket: WebSocket):
    await websocket.accept()
    
    # Get language from query params
    language = websocket.query_params.get("language", "en")
    if language == "auto":
        language = None
    
    print(f"Streaming connection established, language: {language}")
    
    transcriber = StreamingTranscriber(language=language)
    
    # Start processing task
    process_task = asyncio.create_task(transcriber.process_streaming(websocket))
    
    try:
        while True:
            # Receive audio data
            data = await websocket.receive_bytes()
            
            if len(data) == 0:
                continue
            
            # Convert bytes to float32 PCM
            audio_chunk = np.frombuffer(data, dtype=np.int16).astype(np.float32) / 32768.0
            
            # Add to transcriber
            transcriber.add_audio(audio_chunk)
            
            # Log occasionally
            if len(transcriber.audio_buffer) % 160000 == 0:  # Every 10 seconds
                print(f"Received audio: total {len(transcriber.audio_buffer)} samples")
            
    except WebSocketDisconnect:
        print("Client disconnected")
        process_task.cancel()
        
        # Return full recording audio for final processing
        full_audio = np.array(list(transcriber.audio_buffer), dtype=np.float32)
        
        # Save for final processing (or return it)
        # For now, just log
        print(f"Session ended. Collected {len(full_audio)} samples ({len(full_audio)/SAMPLE_RATE:.1f}s)")
        print(f"Finalized {len(transcriber.finalized_segments)} segments")
        
    except Exception as e:
        print(f"Error: {e}")
        import traceback
        traceback.print_exc()
        process_task.cancel()

@app.get("/health")
async def health():
    return {"status": "ok", "device": DEVICE, "model": MODEL_SIZE}

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8003)
