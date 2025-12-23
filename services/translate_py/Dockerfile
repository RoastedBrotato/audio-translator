FROM python:3.11-slim

WORKDIR /app

# Copy requirements and install Python packages
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application code
COPY app.py .

# Expose port
EXPOSE 8004

# Run the service
CMD ["uvicorn", "app:app", "--host", "0.0.0.0", "--port", "8004"]
