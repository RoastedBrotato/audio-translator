#!/bin/bash
# Start the ASR Python service

cd "$(dirname "$0")/services/asr_py"

# Create virtual environment if it doesn't exist
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
    echo ""
fi

# Activate virtual environment
source venv/bin/activate

# Install dependencies
echo "Installing Python dependencies..."
pip install -r requirements.txt

echo ""
echo "Starting ASR service on http://127.0.0.1:8003"
echo "Press Ctrl+C to stop"
echo ""

uvicorn app:app --host 127.0.0.1 --port 8003 --reload
