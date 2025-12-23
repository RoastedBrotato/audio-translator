#!/bin/bash
# Start the Go WebSocket server

cd "$(dirname "$0")"

echo "Starting Go server on http://localhost:8080"
echo "Make sure the ASR service is running on http://127.0.0.1:8003"
echo "Press Ctrl+C to stop"
echo ""

go run cmd/server/main.go
