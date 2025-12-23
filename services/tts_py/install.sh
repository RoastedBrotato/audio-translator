#!/bin/bash
cd "$(dirname "$0")"
rm -rf venv
python3 -m venv venv
./venv/bin/pip install --upgrade pip
./venv/bin/pip install fastapi 'uvicorn[standard]' gtts python-multipart elevenlabs
echo "Installation complete! Run with: ./venv/bin/uvicorn app:app --host 0.0.0.0 --port 8005"
