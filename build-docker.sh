#!/bin/bash
# Build script with Docker BuildKit enabled for better caching

# Enable BuildKit for better caching and parallel builds
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

echo "Building Docker images with BuildKit caching enabled..."

# Build all services
docker compose build "$@"

echo "Build complete! Images are tagged and ready to use."
