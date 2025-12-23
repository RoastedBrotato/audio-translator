#!/bin/bash
# Quick test to verify the ASR service is working

cd "$(dirname "$0")"

echo "Testing ASR service..."
echo ""

# Generate a simple 1-second test WAV file (silence at 16kHz)
python3 -c "
import wave
import struct
import sys

# Create 1 second of silence at 16kHz
sample_rate = 16000
duration = 1
samples = [0] * (sample_rate * duration)

with wave.open('/tmp/test.wav', 'wb') as wf:
    wf.setnchannels(1)
    wf.setsampwidth(2)
    wf.setframerate(sample_rate)
    for s in samples:
        wf.writeframes(struct.pack('<h', s))

print('Created test WAV file')
"

# Test the ASR endpoint
echo ""
echo "Sending test audio to ASR service..."
RESPONSE=$(curl -s -X POST http://127.0.0.1:8003/transcribe \
  -H "Content-Type: audio/wav" \
  --data-binary @/tmp/test.wav)

echo "Response: $RESPONSE"
echo ""

if [ -z "$RESPONSE" ]; then
    echo "❌ No response from ASR service. Is it running?"
    exit 1
else
    echo "✓ ASR service is responding"
fi

# Clean up
rm -f /tmp/test.wav
