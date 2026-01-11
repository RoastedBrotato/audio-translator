# syntax=docker/dockerfile:1.4

FROM python:3.11-slim

WORKDIR /app

# Copy requirements and install dependencies
COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY app.py .

# Expose port
EXPOSE 8007

# Run the service
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8007"]
