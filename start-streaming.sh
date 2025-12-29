#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
    export $(grep -v '^#' .env | xargs)
fi

echo "Starting Audio Translator with Streaming Support..."
echo "=================================================="
echo ""

# Stop old ASR service
docker stop audio-translator-asr 2>/dev/null
docker rm audio-translator-asr 2>/dev/null

# Build new streaming ASR image
echo "Building streaming ASR service with GPU support..."
cd services/asr_streaming
docker build -t asr-streaming:latest .

# Start streaming ASR service with GPU
echo "Starting streaming ASR service on port 8003..."
docker run -d \
  --name asr-streaming \
  --gpus all \
  -p 8003:8003 \
  -e HF_TOKEN="${HF_TOKEN}" \
  --restart unless-stopped \
  asr-streaming:latest

cd ../..

# Wait for ASR to be ready
echo "Waiting for ASR service to load model..."
sleep 10

# Check if translation and TTS are running
if ! docker ps | grep -q audio-translator-translate; then
  echo "Starting translation service..."
  cd services/translate_py
  docker-compose up -d
  cd ../..
fi

# Restart Go server
echo "Starting Go server on port 8080..."
pkill -f "bin/server" 2>/dev/null
./bin/server > /tmp/go-server.log 2>&1 &

sleep 2

# Check services
echo ""
echo "Service Status:"
echo "==============="
curl -s http://localhost:8003/health | jq '.' 2>/dev/null || echo "ASR: Starting..."
curl -s http://localhost:8004/ 2>/dev/null && echo "Translation: ✓" || echo "Translation: ✗"
curl -s http://localhost:8080 > /dev/null 2>&1 && echo "Go Server: ✓" || echo "Go Server: ✗"

echo ""
echo "✓ All services started!"
echo ""
echo "Open: http://localhost:8080/streaming.html"
echo "Logs: tail -f /tmp/go-server.log"
echo "ASR Logs: docker logs -f asr-streaming"
