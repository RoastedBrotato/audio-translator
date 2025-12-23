#!/bin/bash

echo "====================================="
echo "Starting Audio Translator with Docker"
echo "====================================="

# Stop any existing containers
echo "Stopping existing containers..."
docker compose down 2>/dev/null

# Build and start services
echo "Building and starting services..."
docker compose up -d --build

# Wait for services to be ready
echo ""
echo "Waiting for services to start..."
sleep 5

# Check service health
echo ""
echo "Checking service health..."
for i in {1..30}; do
    if curl -s http://localhost:8003/ >/dev/null 2>&1 && \
       curl -s http://localhost:8004/ >/dev/null 2>&1 && \
       curl -s http://localhost:8005/ >/dev/null 2>&1; then
        echo "✓ All Python services are ready!"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "⚠ Services may still be starting. Check logs with: docker-compose logs"
    else
        sleep 2
    fi
done

# Start Go server
echo ""
echo "Starting Go server..."
go run cmd/server/main.go &
GO_PID=$!

sleep 2

echo ""
echo "====================================="
echo "✓ All services are running!"
echo "====================================="
echo ""
echo "Open http://localhost:8080 in your browser"
echo ""
echo "Docker services:"
echo "  docker compose logs -f    # View all logs"
echo "  docker compose down       # Stop services"
echo ""
echo "Go server PID: $GO_PID"
echo "To stop Go server: kill $GO_PID"
echo ""
echo "Press Ctrl+C to stop..."
wait $GO_PID
