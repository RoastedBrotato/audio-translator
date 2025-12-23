#!/bin/bash
cd services/tts_py
if [ ! -d "venv" ]; then
    echo "Creating virtual environment..."
    python3 -m venv venv
fi

source venv/bin/activate
echo "Installing dependencies..."
pip install -q -r requirements.txt

echo "Starting TTS service on port 8005..."
python3 app.py
