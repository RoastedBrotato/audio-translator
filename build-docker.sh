#!/bin/bash
# Build Docker images for all audio-translator services
#
# WHAT THIS DOES:
# - Builds ASR, Translation, and TTS Docker images with BuildKit caching
# - Uses layer caching to skip unchanged steps (much faster on rebuilds)
# - Does NOT start containers (use start-services.sh for that)
#
# WHEN TO USE:
# - After modifying Dockerfiles or Python code
# - To rebuild specific services: ./build-docker.sh asr_streaming
# - To force rebuild without cache: ./build-docker.sh --no-cache
#
# WHAT TO EXPECT:
# - First build: 15-30 minutes (downloads models, base images)
# - Subsequent builds: 1-5 minutes (uses cache for unchanged layers)
# - BuildKit will show parallel build progress for all services
#
# AFTER BUILD:
# - Images are tagged as: asr_streaming:latest, translate_py:latest, tts_py:latest
# - Model cache volumes persist between rebuilds (no re-downloads)
# - Run './start-services.sh' to start containers with new images
#
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
