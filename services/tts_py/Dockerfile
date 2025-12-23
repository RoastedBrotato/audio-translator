FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements and install Python packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
RUN pip install --no-cache-dir TTS==0.22.0

# Copy application code
COPY app.py .

# Set environment variable to bypass TTS CPML license prompt
ENV COQUI_TOS_AGREED=1

# Expose port
EXPOSE 8005

# Run the service
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8005"]
