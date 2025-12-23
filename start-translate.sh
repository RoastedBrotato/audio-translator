#!/bin/bash

cd "$(dirname "$0")/services/translate_py"

# Create venv if it doesn't exist
if [ ! -d "venv" ]; then
    echo "Creating Python virtual environment..."
    python3 -m venv venv
fi

# Activate venv
source venv/bin/activate

# Install dependencies
echo "Installing Python dependencies..."
pip install -q -r requirements.txt

echo ""
echo "Starting Translation service on http://127.0.0.1:8004"
echo "Press Ctrl+C to stop"
echo ""

# Run the service and redirect output to log
uvicorn app:app --host 127.0.0.1 --port 8004 2>&1 | tee /tmp/translate.log
