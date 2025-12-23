#!/bin/bash
# Combined startup script for both services

PROJECT_ROOT="$(cd "$(dirname "$0")" && pwd)"

echo "====================================="
echo "Starting Realtime Caption Translator"
echo "====================================="
echo ""

# Kill any existing processes
echo "Cleaning up any existing processes..."
pkill -f "uvicorn app:app" || true
pkill -f "go run.*main.go" || true
sleep 1

# Start ASR service in background
echo "Starting ASR service..."
cd "$PROJECT_ROOT/services/asr_py"

if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
fi

source venv/bin/activate
pip install -q -r requirements.txt

echo "ASR service starting on http://127.0.0.1:8003"
uvicorn app:app --host 127.0.0.1 --port 8003 > /tmp/asr.log 2>&1 &
ASR_PID=$!
echo "ASR PID: $ASR_PID"

# Wait for ASR service to be ready
echo "Waiting for ASR service to be ready..."
for i in {1..30}; do
    if curl -s http://127.0.0.1:8003 > /dev/null 2>&1; then
        echo "✓ ASR service is ready"
        break
    fi
    sleep 1
done

# Start Translation service in background
echo ""
echo "Starting Translation service..."
cd "$PROJECT_ROOT/services/translate_py"

if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
fi

source venv/bin/activate
pip install -q -r requirements.txt

echo "Translation service starting on http://127.0.0.1:8004"
uvicorn app:app --host 127.0.0.1 --port 8004 > /tmp/translate.log 2>&1 &
TRANSLATE_PID=$!
echo "Translation PID: $TRANSLATE_PID"

# Wait for Translation service to be ready
echo "Waiting for Translation service to be ready..."
for i in {1..10}; do
    if curl -s http://127.0.0.1:8004 > /dev/null 2>&1; then
        echo "✓ Translation service is ready"
        break
    fi
    sleep 1
done

# Start TTS service in background
echo ""
echo "Starting TTS service..."
cd "$PROJECT_ROOT/services/tts_py"

if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
fi

source venv/bin/activate
pip install -q -r requirements.txt

echo "TTS service starting on http://127.0.0.1:8005"
uvicorn app:app --host 127.0.0.1 --port 8005 > /tmp/tts.log 2>&1 &
TTS_PID=$!
echo "TTS PID: $TTS_PID"

# Wait for TTS service to be ready
echo "Waiting for TTS service to be ready..."
for i in {1..10}; do
    if curl -s http://127.0.0.1:8005/health > /dev/null 2>&1; then
        echo "✓ TTS service is ready"
        break
    fi
    sleep 1
done

# Start Go server in background
echo ""
echo "Starting Go WebSocket server..."
cd "$PROJECT_ROOT"
go run cmd/server/main.go > /tmp/go-server.log 2>&1 &
GO_PID=$!
echo "Go server PID: $GO_PID"

# Wait for Go server to be ready
echo "Waiting for Go server to be ready..."
for i in {1..10}; do
    if curl -s http://localhost:8080 > /dev/null 2>&1; then
        echo "✓ Go server is ready"
        break
    fi
    sleep 1
done

echo ""
echo "====================================="
echo "✓ All services are running!"
echo "====================================="
echo ""
echo "Open http://localhost:8080 in your browser"
echo ""
echo "Logs:"
echo "  ASR service:         tail -f /tmp/asr.log"
echo "  Translation service: tail -f /tmp/translate.log"
echo "  TTS service:         tail -f /tmp/tts.log"
echo "  Go server:           tail -f /tmp/go-server.log"
echo ""
echo "To stop services:"
echo "  kill $ASR_PID $TRANSLATE_PID $TTS_PID $GO_PID"
echo ""
echo "Press Ctrl+C to stop all services..."

# Wait and cleanup on exit
trap "kill $ASR_PID $TRANSLATE_PID $TTS_PID $GO_PID 2>/dev/null" EXIT
wait
