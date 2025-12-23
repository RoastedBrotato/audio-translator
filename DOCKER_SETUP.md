# Docker Setup Guide for Audio Translator

## The XTTS Model Download Issue

**Why does it keep downloading?**

The XTTS v2 model (~1.8GB) needs to be downloaded on first run. If the container crashes or restarts during/after download, it may re-download because:

1. **Syntax errors** in the code cause container crashes (NOW FIXED ✅)
2. **PyTorch version incompatibility** causes load failures (NOW FIXED ✅)
3. **TTS library update checking** can trigger cache clearing

## Solution: Let It Download ONCE

The fixed version will now:
- ✅ Use gTTS as fallback immediately (app works right away)
- ✅ Download XTTS in background (takes 2-5 minutes)
- ✅ Cache the model in Docker volume (won't re-download)
- ✅ Not crash during or after download

## How to Launch

### Step 1: Start Docker Services
```bash
docker compose up -d
```

### Step 2: Monitor TTS Download (Optional)
```bash
# Watch progress
docker logs -f audio-translator-tts

# Or check if ready
./check-services.sh
```

### Step 3: Start Go Server
```bash
# Wait until TTS shows "Application startup complete" OR you see gTTS messages
# Then start:
go run cmd/server/main.go
```

### Step 4: Open Application
```bash
# Open in browser:
http://localhost:8080
```

## The app will work IMMEDIATELY with gTTS!

Even while XTTS is downloading, your application will work using Google Text-to-Speech. Once XTTS finishes loading, it will automatically switch to using that for better quality.

## Troubleshooting

### Container keeps restarting?
```bash
# Check for errors
docker logs audio-translator-tts

# If stuck, stop and restart
docker compose down
docker compose up -d
```

### Want to force re-download?
```bash
# Delete the volume
docker compose down
docker volume rm audio-translator_tts-models
docker compose up -d
```

### Check what's using disk space
```bash
docker system df -v
```

## Quick Commands

```bash
# Start everything
docker compose up -d && go run cmd/server/main.go

# Check status
./check-services.sh

# View logs
docker compose logs -f

# Stop everything
docker compose down
pkill -f "go run.*main.go"

# Restart just TTS
docker compose restart tts
```
