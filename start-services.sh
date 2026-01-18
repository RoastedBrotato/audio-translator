#!/bin/bash
# Start all audio-translator services and the Go web server
#
# WHAT THIS DOES:
# - Checks if Docker images exist, builds them if missing (first-time setup)
# - Starts ASR, Translation, and TTS containers in background
# - Builds and starts the Go web server on port 8080
# - Shows service status and URLs
#
# WHEN TO USE:
# - First time setup (will build everything automatically)
# - After a reboot or when containers are stopped
# - When you want to restart all services
#
# WHAT TO EXPECT:
# - If images exist: Starts in ~10-30 seconds
# - If images need building: 15-30 minutes for first build
# - ASR container takes ~60 seconds to load models and become healthy
# - TTS container takes ~40 seconds to load XTTS v2 model
# - All containers will auto-restart if they crash
#
# IMPORTANT:
# - This script does NOT rebuild images if they already exist
# - If you modified code, run './build-docker.sh' first, then this script
# - Model caches are preserved in Docker volumes (no re-downloads)
# - Safe to run multiple times - will detect and handle already-running containers
# - Automatically detects and stops old standalone containers that conflict
#
# TO STOP:
# - Stop all: docker compose down && pkill -f bin/server
# - Stop just Docker: docker compose down
# - Stop just web server: kill $(cat bin/server.pid)
#
set -euo pipefail

ROOT_DIR=$(cd "$(dirname "$0")" && pwd)
cd "$ROOT_DIR"

echo "🚀 Starting Audio Translator Services..."
echo ""

# Check if system Ollama service is using port 11434
if lsof -Pi :11434 -sTCP:LISTEN -t >/dev/null 2>&1 || ss -ltn | grep -q ':11434 '; then
    if systemctl is-active --quiet ollama 2>/dev/null; then
        echo "⚠️  System Ollama service detected on port 11434"
        echo "   Stopping it to avoid conflicts with Docker container..."
        sudo systemctl stop ollama
        echo "✓ System Ollama stopped"
        echo ""
    fi
fi

# Enable BuildKit
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Check and configure .env file with required variables
if [ ! -f .env ]; then
    echo "⚠️  .env file not found, creating with default values..."
    touch .env
fi

# Ensure required database variables are set
if ! grep -q "^DB_HOST=" .env; then
    echo "📝 Adding database configuration to .env..."
    cat >> .env << 'EOF'

# Database configuration
DB_HOST=localhost
DB_PORT=5433
DB_USER=audio_translator
DB_PASSWORD=audio_translator_pass
DB_NAME=audio_translator
EOF
fi

# Ensure MinIO variables are set
if ! grep -q "^MINIO_ENABLED=" .env; then
    echo "📝 Adding MinIO configuration to .env..."
    cat >> .env << 'EOF'

# MinIO configuration
MINIO_ENABLED=true
MINIO_ENDPOINT=localhost:9000
MINIO_ROOT_USER=minioadmin
MINIO_ROOT_PASSWORD=minioadmin123
MINIO_BUCKET=audio-translator-files
MINIO_USE_SSL=false
EOF
fi

# Ensure LLM service URL is set
if ! grep -q "^LLM_BASE_URL=" .env; then
    echo "📝 Adding LLM service URL to .env..."
    cat >> .env << 'EOF'

# LLM Service
LLM_BASE_URL=http://127.0.0.1:8007
EOF
fi

echo "✓ Environment configuration verified"
echo ""

# Check for old standalone containers that might conflict with docker-compose
OLD_CONTAINERS=$(docker ps --format "{{.Names}}" | grep -E "^(asr-streaming|audio-translator-asr|audio-translator-translate|audio-translator-tts)$" || true)

if [ -n "$OLD_CONTAINERS" ]; then
    echo "⚠️  Found old standalone containers that will conflict with docker-compose:"
    echo "$OLD_CONTAINERS"
    echo ""
    read -p "Stop these old containers? [Y/n] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]] || [[ -z $REPLY ]]; then
        echo "⏹️  Stopping old containers..."
        echo "$OLD_CONTAINERS" | xargs docker stop
        echo "✓ Old containers stopped"
        echo ""
    else
        echo "❌ Cannot proceed with conflicting containers running."
        echo "   Please stop them manually: docker stop $OLD_CONTAINERS"
        exit 1
    fi
fi

# Check if images exist, if not build them
if ! docker images | grep -q "asr_streaming.*latest"; then
    echo "📦 Images not found. Building for the first time (this will take a while)..."
    docker compose build
    echo ""
fi

# Start services (this is idempotent - won't fail if already running)
echo "▶️  Starting containers..."
if ! docker compose up -d; then
    echo ""
    echo "❌ Failed to start containers. Check for port conflicts:"
    echo "   Ports in use: $(docker ps --format '{{.Names}}\t{{.Ports}}' | grep -E '8003|8004|8005|8080')"
    exit 1
fi

# Ensure all containers are properly networked
echo "🔗 Verifying container networking..."
OLLAMA_NETWORK=$(docker inspect audio-translator-ollama-1 -f '{{range $k, $v := .NetworkSettings.Networks}}{{$k}}{{end}}' 2>/dev/null || echo "")
if [ -z "$OLLAMA_NETWORK" ] || [ "$OLLAMA_NETWORK" != "audio-translator_default" ]; then
    echo "   Connecting Ollama to network..."
    docker network connect audio-translator_default audio-translator-ollama-1 2>/dev/null || true
fi

# Wait for services to initialize
sleep 3

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
# Load environment variables from .env file
set -a
source .env
set +a
nohup "$SERVER_BIN" > "$SERVER_LOG" 2>&1 &
echo $! > "$SERVER_PID_FILE"

echo ""
echo "� Verifying services..."
# Check if LLM service can reach Ollama
for i in {1..5}; do
    if docker exec audio-translator-llm_service-1 python -c "import urllib.request; urllib.request.urlopen('http://ollama:11434/api/tags', timeout=2)" >/dev/null 2>&1; then
        echo "✓ LLM service can reach Ollama"
        break
    fi
    if [ $i -eq 5 ]; then
        echo "⚠️  Warning: LLM service cannot reach Ollama yet. Chat may not work immediately."
        echo "   This usually resolves after a few seconds. Try restarting if chat doesn't work:"
        echo "   docker restart audio-translator-llm_service-1"
    fi
    sleep 2
done

echo ""
echo "📊 Service URLs:"
echo "  - Web UI:         http://localhost:8080"
echo "  - ASR Streaming:  http://localhost:8003"
echo "  - Translation:    http://localhost:8004"
echo "  - TTS:            http://localhost:8005"
echo "  - LLM Service:    http://localhost:8007"
echo ""
echo "💡 Commands:"
echo "  - View container logs: docker compose logs -f"
echo "  - View web server log: tail -f $SERVER_LOG"
echo "  - Stop services:        docker compose down && pkill -f $SERVER_BIN"
echo "  - Rebuild images:       ./build-docker.sh && docker compose up -d"
