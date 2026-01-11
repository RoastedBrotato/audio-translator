# syntax=docker/dockerfile:1.4

# Build stage - includes compilers for building Python packages
FROM python:3.11-slim AS builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install to local user directory
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --user -r requirements.txt

# Pre-download the embedding model during build
COPY preload_models.py .
RUN python3 preload_models.py

# Runtime stage - minimal image without build tools
FROM python:3.11-slim

WORKDIR /app

# Copy installed packages from builder stage
COPY --from=builder /root/.local /root/.local
COPY --from=builder /root/.cache/huggingface /root/.cache/huggingface

# Copy application code
COPY app.py .

# Add local packages to PATH
ENV PATH=/root/.local/bin:$PATH

# Expose port
EXPOSE 8006

# Run the service
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8006"]
