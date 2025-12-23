#!/bin/bash
# Quick check of Docker service status

echo "Docker Services Status:"
echo "======================="
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep audio-translator

echo ""
echo "Service Health:"
echo "==============="

# Check ASR
if curl -s http://127.0.0.1:8003 > /dev/null 2>&1; then
    echo "✓ ASR (8003): Ready"
else
    echo "✗ ASR (8003): Not ready"
fi

# Check Translation
if curl -s http://127.0.0.1:8004 > /dev/null 2>&1; then
    echo "✓ Translation (8004): Ready"
else
    echo "✗ Translation (8004): Not ready"
fi

# Check TTS
if curl -s http://127.0.0.1:8005/health > /dev/null 2>&1; then
    echo "✓ TTS (8005): Ready"
else
    echo "⏳ TTS (8005): Still loading (check: docker logs audio-translator-tts)"
fi

echo ""
echo "To view logs: docker logs -f audio-translator-tts"
echo "To start Go server: go run cmd/server/main.go"
