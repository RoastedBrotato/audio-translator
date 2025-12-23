#!/bin/bash
# Start the complete application using Docker services + Go server

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "====================================="
echo "Starting Audio Translator (Docker)"
echo "====================================="
echo ""

# Start Docker services
echo "Starting Docker services (ASR, Translation, TTS)..."
docker compose up -d

echo ""
echo "Waiting for services to be ready..."
echo "⏳ TTS service may take 2-3 minutes to load XTTS v2 model..."
echo ""

# Wait for services with better feedback
services=("ASR:8003" "Translation:8004" "TTS:8005")
for service in "${services[@]}"; do
    name="${service%:*}"
    port="${service#*:}"
    echo -n "Waiting for $name service (port $port)... "
    
    max_attempts=180  # 3 minutes for TTS
    attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if curl -s http://127.0.0.1:$port > /dev/null 2>&1; then
            echo "✓ Ready"
            break
        fi
        sleep 1
        attempt=$((attempt + 1))
        
        # Show progress for TTS
        if [ "$name" = "TTS" ] && [ $((attempt % 10)) -eq 0 ]; then
            echo -n "."
        fi
    done
    
    if [ $attempt -eq $max_attempts ]; then
        echo "⚠ Timeout (check logs: docker logs audio-translator-${name,,})"
    fi
done

# Start Go WebSocket server
echo ""
echo "Starting Go WebSocket server..."
cd "$PROJECT_ROOT"
go run cmd/server/main.go > /tmp/go-server.log 2>&1 &
GO_PID=$!
echo "Go server PID: $GO_PID"

# Wait for Go server
echo -n "Waiting for Go server (port 8080)... "
for i in {1..30}; do
    if curl -s http://127.0.0.1:8080 > /dev/null 2>&1; then
        echo "✓ Ready"
        break
    fi
    sleep 1
done

echo ""
echo "====================================="
echo "✓ Application is running!"
echo "====================================="
echo ""
echo "Open http://localhost:8080 in your browser"
echo ""
echo "Logs:"
echo "  Docker services: docker compose logs -f"
echo "  Go server:       tail -f /tmp/go-server.log"
echo ""
echo "To stop:"
echo "  docker compose down"
echo "  kill $GO_PID"
echo ""
