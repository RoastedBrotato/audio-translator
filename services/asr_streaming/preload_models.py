#!/usr/bin/env python3
"""Pre-download models during Docker build to avoid re-downloading on every container start"""

import torch
import whisper

print("=" * 60)
print("PRE-DOWNLOADING MODELS FOR DOCKER IMAGE")
print("=" * 60)

# Pre-download Whisper medium model
print("\n1. Downloading Whisper medium model...")
MODEL_SIZE = "medium"
# Always use CPU during build - GPU not available at build time
DEVICE = "cpu"
print(f"   Device: {DEVICE} (GPU will be used at runtime)")
whisper_model = whisper.load_model(MODEL_SIZE, device=DEVICE)
print(f"   ✓ Whisper {MODEL_SIZE} model downloaded successfully")

# Pre-download Silero VAD model
print("\n2. Downloading Silero VAD model...")
vad_model, utils = torch.hub.load(
    repo_or_dir='snakers4/silero-vad',
    model='silero_vad',
    force_reload=False
)
print("   ✓ Silero VAD model downloaded successfully")

# Try to pre-download pyannote model (optional - may fail without HF token)
print("\n3. Attempting to download pyannote speaker diarization model...")
try:
    from pyannote.audio import Pipeline
    diarization_pipeline = Pipeline.from_pretrained(
        "pyannote/speaker-diarization-3.1",
        use_auth_token=None
    )
    print("   ✓ Pyannote diarization model downloaded successfully")
except Exception as e:
    print(f"   ⚠ Could not download pyannote model: {e}")
    print("   → This is optional. Speaker diarization will try to download on first use.")

print("\n" + "=" * 60)
print("MODEL PRE-DOWNLOAD COMPLETE")
print("=" * 60)
