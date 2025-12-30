# syntax=docker/dockerfile:1.4
# Use runtime image instead of devel - much smaller and more secure
FROM nvidia/cuda:12.0.0-cudnn8-runtime-ubuntu22.04

# Install Python and runtime dependencies
RUN apt-get update && apt-get install -y \
    python3.10 \
    python3-pip \
    ffmpeg \
    git \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy requirements
COPY requirements.txt .

# Install Python packages with BuildKit cache mount
RUN --mount=type=cache,target=/root/.cache/pip \
    pip3 install -r requirements.txt

# Copy application files
COPY app.py .
COPY preload_models.py .

# Pre-download models during build (caches them in the Docker image)
RUN python3 preload_models.py

# Create non-root user for security
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app /root/.cache

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8003

# Run the application
CMD ["python3", "app.py"]
