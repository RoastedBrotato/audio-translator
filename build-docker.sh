#!/bin/bash
# Build script with Docker BuildKit enabled for better caching
set -euo pipefail

# Enable BuildKit for better caching and parallel builds
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Verify docker is available
if ! command -v docker >/dev/null 2>&1; then
    echo "❌ Error: docker not found. Please install Docker and try again." >&2
    exit 1
fi

# Verify docker-compose is available
if ! docker compose version >/dev/null 2>&1; then
    echo "❌ Error: docker compose not available. Please install Docker Compose v2." >&2
    exit 1
fi

echo "🔨 Building Docker images with BuildKit caching enabled..."
echo ""

# Build all services with progress output
if docker compose build "$@"; then
    echo ""
    echo "✅ Build complete! Images are tagged and ready to use."
    echo ""
    echo "📊 Image sizes:"
    docker images | grep -E "REPOSITORY|asr_streaming|translate_py|tts_py" | head -4
    echo ""
    echo "💡 Next: Run './start-services.sh' to start all services"
else
    echo ""
    echo "❌ Build failed! Check the error messages above."
    exit 1
fi
