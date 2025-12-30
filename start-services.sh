#!/bin/bash
# Start all audio-translator services and the Go web server

set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$ROOT_DIR"

echo "🚀 Starting Audio Translator Services..."
echo ""

# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Check if images exist, if not build them
if ! docker images | grep -q "asr_streaming.*latest"; then
    echo "📦 Images not found. Building for the first time (this will take a while)..."
    docker compose build
    echo ""
fi

# Start services
echo "▶️  Starting containers..."
docker compose up -d

# Wait a moment for services to initialize
sleep 2

# Show status
echo ""
echo "✅ Services Status:"
docker compose ps

# Build and start Go web/API server (serves main page at :8080)
echo ""
echo "🔧 Building Go server..."
if ! command -v go >/dev/null 2>&1; then
    echo "Go is required to run the web server. Please install Go and re-run." >&2
    exit 1
fi

mkdir -p "$ROOT_DIR/bin"
SERVER_BIN="$ROOT_DIR/bin/server"
SERVER_LOG="$ROOT_DIR/bin/server.log"
SERVER_PID_FILE="$ROOT_DIR/bin/server.pid"

# Stop any existing server instance
if [ -f "$SERVER_PID_FILE" ] && kill -0 "$(cat "$SERVER_PID_FILE")" 2>/dev/null; then
    echo "⏹️  Stopping existing server (pid $(cat "$SERVER_PID_FILE"))"
    kill "$(cat "$SERVER_PID_FILE")" || true
    sleep 1
fi

go build -o "$SERVER_BIN" ./cmd/server

echo "▶️  Starting Go server on http://localhost:8080"
nohup "$SERVER_BIN" > "$SERVER_LOG" 2>&1 &
echo $! > "$SERVER_PID_FILE"

echo ""
echo "📊 Service URLs:"
echo "  - Web UI:         http://localhost:8080"
echo "  - ASR Streaming:  http://localhost:8003"
echo "  - Translation:    http://localhost:8004"
echo "  - TTS:            http://localhost:8005"
echo ""
echo "💡 Commands:"
echo "  - View container logs: docker compose logs -f"
echo "  - View web server log: tail -f $SERVER_LOG"
echo "  - Stop services:        docker compose down && pkill -f $SERVER_BIN"
echo "  - Rebuild images:       ./build-docker.sh && docker compose up -d"
