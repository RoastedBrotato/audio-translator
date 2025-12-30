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
    pip install --user -r requirements.txt && \
    pip install --user TTS==0.22.0

# Runtime stage - minimal image without build tools
FROM python:3.11-slim

WORKDIR /app

# Copy installed packages from builder stage
COPY --from=builder /root/.local /root/.local

# Copy application code
COPY app.py .

# Add local packages to PATH
ENV PATH=/root/.local/bin:$PATH

# Set environment variable to bypass TTS CPML license prompt
ENV COQUI_TOS_AGREED=1

# Create non-root user for security
RUN useradd -m -u 1000 appuser && \
    chown -R appuser:appuser /app /root/.local

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8005

# Run the service
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8005"]
