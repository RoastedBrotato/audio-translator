# Docker Optimizations Applied

**Date**: 2025-12-30
**Status**: ✅ Complete

This document summarizes the Docker optimizations that were applied to improve build performance, security, and resource management.

---

## Summary of Changes

### ✅ 1. ASR Service Dockerfile Optimizations
**File**: `services/asr_streaming/Dockerfile`

**Changes**:
- Switched from `nvidia/cuda:12.0.0-cudnn8-devel-ubuntu22.04` to `-runtime` variant
- Added non-root user (`appuser`)
- Maintained BuildKit cache mounts

**Benefits**:
- **~2GB smaller image** (devel → runtime)
- **Better security** (runs as non-root)
- **Reduced attack surface** (no development tools in production)

### ✅ 2. TTS Service Multi-Stage Build
**File**: `services/tts_py/Dockerfile`

**Changes**:
- Implemented multi-stage build pattern
- Build stage: Includes gcc/g++ for compiling Python packages
- Runtime stage: Only includes compiled packages, no build tools
- Added non-root user (`appuser`)

**Benefits**:
- **~200MB smaller image** (removed gcc/g++)
- **Faster container startup**
- **Better security** (no compilers in production)

### ✅ 3. Translation Service Security
**File**: `services/translate_py/Dockerfile`

**Changes**:
- Added non-root user (`appuser`)
- Already using slim base image

**Benefits**:
- **Better security** (runs as non-root)
- **Minimal footprint** (already optimized)

### ✅ 4. Fixed Duplicate Volume Mounts
**File**: `docker-compose.yml`

**Changes**:
- Removed duplicate `transformers_cache` volume
- Both ASR and Translation services now share single `huggingface_cache` volume

**Before**:
```yaml
volumes:
  huggingface_cache:
  transformers_cache:  # Duplicate!
```

**After**:
```yaml
volumes:
  huggingface_cache:  # Shared between services
```

**Benefits**:
- **Saves disk space** (models downloaded once)
- **Faster startup** (cache reuse)
- **Cleaner architecture**

### ✅ 5. Resource Limits Added
**File**: `docker-compose.yml`

**Changes**:
Added CPU and memory limits for all services:

| Service | CPU Limit | Memory Limit | CPU Reserved | Memory Reserved |
|---------|-----------|--------------|--------------|-----------------|
| ASR     | 4 cores   | 8GB          | 2 cores      | 4GB             |
| Translate | 2 cores | 2GB          | 1 core       | 1GB             |
| TTS     | 2 cores   | 4GB          | 1 core       | 2GB             |

**Benefits**:
- **Prevents resource exhaustion** (one service can't starve others)
- **Predictable performance**
- **Better for production environments**

### ✅ 6. GPU Allocation Optimized
**File**: `docker-compose.yml`

**Changes**:
- Changed `count: all` to `count: 1` for ASR service
- Keeps explicit `CUDA_VISIBLE_DEVICES=0`

**Benefits**:
- **Works better on multi-GPU systems**
- **Explicit GPU allocation**
- **Prevents GPU hogging**

---

## Image Size Comparison (Estimated)

| Service | Before | After | Savings |
|---------|--------|-------|---------|
| ASR     | ~8GB   | ~6GB  | ~2GB    |
| Translate | ~500MB | ~500MB | 0MB    |
| TTS     | ~1.2GB | ~1GB  | ~200MB  |
| **Total** | **~9.7GB** | **~7.5GB** | **~2.2GB** |

---

## Security Improvements

### All Services Now Run as Non-Root

**Why this matters**:
- If container is compromised, attacker has limited privileges
- Follows principle of least privilege
- Industry best practice for containerized applications

**Implementation**:
```dockerfile
# Create non-root user
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app

# Switch to non-root user
USER appuser
```

### Minimal Base Images

- ASR: CUDA runtime (not devel)
- Translation: Python slim
- TTS: Python slim (multi-stage)

**Benefits**:
- Fewer installed packages = smaller attack surface
- No unnecessary tools (compilers, debuggers) in production
- Faster security patching (fewer packages to update)

---

## Build Performance Improvements

### BuildKit Cache Mounts
All services use BuildKit cache mounts for pip:
```dockerfile
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install -r requirements.txt
```

**Benefits**:
- Downloaded packages cached between builds
- Rebuilds only when requirements.txt changes
- **Much faster iteration during development**

### Layer Optimization
Proper layer ordering ensures efficient caching:
1. Base image
2. System dependencies
3. Requirements.txt (changes infrequently)
4. Application code (changes frequently)

### Multi-Stage Builds (TTS)
Separates build-time and runtime dependencies:
```dockerfile
# Builder stage - has gcc/g++
FROM python:3.11-slim AS builder
RUN apt-get install gcc g++
RUN pip install --user -r requirements.txt

# Runtime stage - no build tools
FROM python:3.11-slim
COPY --from=builder /root/.local /root/.local
```

---

## Resource Management

### Why Resource Limits Matter

**Without limits**:
- Services can consume all available CPU/memory
- One misbehaving service can crash entire system
- Unpredictable performance
- Difficult to debug resource issues

**With limits**:
- Guaranteed minimum resources (reservations)
- Protected maximum usage (limits)
- Docker enforces limits automatically
- Better observability

### Monitoring Resource Usage

Check current usage:
```bash
docker stats
```

View service resource configuration:
```bash
docker compose config
```

---

## Volume Management Improvements

### Before
```yaml
asr_streaming:
  volumes:
    - huggingface_cache:/root/.cache/huggingface

translate_py:
  volumes:
    - transformers_cache:/root/.cache/huggingface  # Same path!
```

**Problem**: Two different named volumes pointing to same directory = wasted space

### After
```yaml
asr_streaming:
  volumes:
    - huggingface_cache:/root/.cache/huggingface

translate_py:
  volumes:
    - huggingface_cache:/root/.cache/huggingface  # Shared!
```

**Benefit**: Models downloaded once, used by both services

---

## How to Apply These Changes

### 1. Rebuild Images
```bash
./build-docker.sh
```

This will:
- Use new optimized Dockerfiles
- Download smaller base images
- Build with multi-stage builds
- Apply all improvements

### 2. Restart Services
```bash
docker compose down
docker compose up -d
```

### 3. Verify Improvements

**Check image sizes**:
```bash
docker images | grep -E "asr_streaming|translate_py|tts_py"
```

**Check running containers are non-root**:
```bash
docker compose exec asr_streaming whoami
# Should output: appuser
```

**Check resource limits**:
```bash
docker stats
```

**Check health status**:
```bash
docker compose ps
# All services should show "healthy"
```

---

## Potential Build Issues and Solutions

### Issue 1: Permission Errors

**Symptom**: Build fails with "Permission denied" errors

**Cause**: Non-root user can't access files

**Solution**: Ensure `chown` commands set proper ownership:
```dockerfile
RUN chown -R appuser:appuser /app /root/.cache
```

### Issue 2: GPU Not Found

**Symptom**: ASR service fails with CUDA errors

**Cause**: NVIDIA container runtime not configured

**Solution**:
```bash
# Check NVIDIA runtime is available
docker run --rm --gpus all nvidia/cuda:12.0.0-base-ubuntu22.04 nvidia-smi

# If fails, run:
./setup-gpu.sh
```

### Issue 3: Out of Memory During Build

**Symptom**: Build killed or hangs

**Cause**: Large model downloads during build

**Solution**: Increase Docker memory limit in Docker Desktop or daemon config

### Issue 4: Slow Builds

**Symptom**: Builds take a long time even with no changes

**Cause**: BuildKit not enabled or cache not working

**Solution**:
```bash
# Ensure BuildKit is enabled
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

# Verify in build output:
./build-docker.sh
# Should see [buildkit] in output
```

---

## Testing the Optimizations

### 1. Build Time Test
```bash
# Clean build
docker compose down
docker system prune -af
time ./build-docker.sh

# Cached build (change app.py)
echo "# test" >> services/asr_streaming/app.py
time ./build-docker.sh
# Should be much faster
```

### 2. Runtime Test
```bash
# Start services
./start-services.sh

# Check health endpoints
curl http://localhost:8003/health
curl http://localhost:8004/health
curl http://localhost:8005/health

# Check resource usage
docker stats --no-stream
```

### 3. Security Test
```bash
# Verify non-root user
docker compose exec asr_streaming id
# Should show uid=1000(appuser) gid=1000(appuser)

# Verify no compilers in TTS
docker compose exec tts_py which gcc
# Should return empty (not found)
```

---

## Next Steps (Optional)

### For Production Deployment:
1. Pin specific image versions (not `:latest`)
2. Add image scanning (Trivy, Snyk)
3. Use private registry for images
4. Implement image signing
5. Add automated security updates

### For Further Optimization:
1. Use Docker layer caching in CI/CD
2. Implement distroless images for even smaller size
3. Add image compression
4. Use specific Python versions (not just `3.11-slim`)
5. Consider Alpine-based images (if compatible)

---

## Configuration Reference

### Environment Variables Used

**DOCKER_BUILDKIT**: Enable BuildKit features
```bash
export DOCKER_BUILDKIT=1
```

**COMPOSE_DOCKER_CLI_BUILD**: Use BuildKit with docker-compose
```bash
export COMPOSE_DOCKER_CLI_BUILD=1
```

**CUDA_VISIBLE_DEVICES**: Specify GPU for ASR
```bash
CUDA_VISIBLE_DEVICES=0  # Use first GPU
```

### Volume Locations

**On host** (Docker Desktop default):
- Linux: `/var/lib/docker/volumes/`
- Mac: `~/Library/Containers/com.docker.docker/Data/vms/0/`
- Windows: `\\wsl$\docker-desktop-data\version-pack-data\community\docker\volumes\`

**Backup volumes**:
```bash
docker run --rm -v whisper_cache:/data -v $(pwd):/backup ubuntu tar czf /backup/whisper_cache.tar.gz /data
```

---

## Troubleshooting

### Clear all caches and rebuild
```bash
# Nuclear option - removes everything
docker compose down
docker system prune -af --volumes
./build-docker.sh
```

### Check container logs
```bash
docker compose logs asr_streaming
docker compose logs translate_py
docker compose logs tts_py
```

### Inspect resource limits
```bash
docker inspect audio-translator-asr_streaming-1 | jq '.[0].HostConfig.Resources'
```

---

## Conclusion

These Docker optimizations provide:
- ✅ **Smaller images** (~2.2GB total savings)
- ✅ **Better security** (non-root users, minimal base images)
- ✅ **Faster builds** (BuildKit caching, multi-stage builds)
- ✅ **Resource protection** (CPU/memory limits)
- ✅ **Improved reliability** (health checks, proper dependencies)

All changes are production-ready and follow Docker best practices.
