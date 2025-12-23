# TTS Service Behavior

## Two Modes of Operation

### 1. gTTS Fallback (Immediate, Basic)
- **When**: While XTTS v2 is downloading/loading or if it fails to load
- **Capabilities**: 
  - ✅ Basic text-to-speech
  - ✅ Multiple languages supported
  - ✅ Works immediately (no waiting)
  - ❌ **NO voice cloning support**
  - ❌ Lower quality audio
- **Endpoints**: `/synthesize` only

### 2. XTTS v2 (After Loading, Advanced)
- **When**: After XTTS v2 model finishes downloading and loading (~3-5 minutes first time)
- **Capabilities**:
  - ✅ High-quality text-to-speech
  - ✅ **Voice cloning from reference audio**
  - ✅ Multiple languages with better pronunciation
  - ✅ Natural-sounding speech
- **Endpoints**: `/synthesize` AND `/synthesize_with_voice`

## How Your Application Handles This

### Video Processing Flow:
1. **User uploads video** with `cloneVoice=true`
2. **If XTTS loaded**: Tries `/synthesize_with_voice` with original audio as reference
3. **If XTTS not loaded or fails**: Automatically falls back to `/synthesize` (gTTS)
4. **Result**: Video always gets TTS audio, but quality depends on XTTS availability

### Current Behavior:
```
XTTS Loading? 
  ├─ Yes → Use gTTS (works but no voice cloning)
  └─ No  → Use XTTS (high quality + voice cloning if requested)
```

## Checking TTS Status

```bash
# Check which mode is active
curl http://127.0.0.1:8005/health

# Response when loading:
{
  "status": "loading",
  "model": "gtts",
  "xtts_loaded": false,
  "fallback_available": true
}

# Response when ready:
{
  "status": "ready",
  "model": "xtts_v2",
  "xtts_loaded": true,
  "fallback_available": true
}
```

## Timeline

### First Run (with Docker volume cache):
- **0:00** - Container starts, service available with gTTS
- **0:00-5:00** - XTTS downloading in background (~1.8GB)
- **5:00+** - XTTS loaded, voice cloning available

### Subsequent Runs:
- **0:00** - Container starts, service available with gTTS
- **0:00-0:30** - XTTS loading from cache (much faster!)
- **0:30+** - XTTS loaded, voice cloning available

## Error Messages

### When Voice Cloning Requested But XTTS Not Ready:
```
503 Service Unavailable
"XTTS v2 model is still loading. Voice cloning will be available soon. 
Please try again in a few minutes or use /synthesize endpoint for basic TTS with gTTS."
```

Your Go server sees this 503 and automatically falls back to `/synthesize` with gTTS.

## Recommendation

For the best user experience:
1. **Let XTTS download once** - takes 3-5 minutes, but cached forever
2. **Inform users** if voice cloning isn't available yet
3. **The fallback works great** for immediate functionality
4. **Voice cloning will work** automatically once XTTS loads

Your code already handles all of this gracefully! 👍
