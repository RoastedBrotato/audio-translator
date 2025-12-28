#!/bin/bash

echo "Installing NVIDIA Container Toolkit for Docker GPU support..."
echo "=============================================================="
echo ""

# Add NVIDIA repository
echo "Adding NVIDIA repository..."
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# Install toolkit
echo "Installing nvidia-container-toolkit..."
sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit

# Configure Docker
echo "Configuring Docker to use NVIDIA runtime..."
sudo nvidia-ctk runtime configure --runtime=docker

# Restart Docker
echo "Restarting Docker..."
sudo systemctl restart docker

# Test GPU access
echo ""
echo "Testing GPU access in Docker..."
docker run --rm --gpus all nvidia/cuda:12.0.0-base-ubuntu22.04 nvidia-smi

echo ""
echo "✓ GPU setup complete!"
echo ""
echo "Now rebuild and restart the ASR service:"
echo "  cd services/asr_streaming"
echo "  docker build -t asr-streaming:latest ."
echo "  docker rm -f asr-streaming"
echo "  docker run -d --name asr-streaming --gpus all -p 8003:8003 asr-streaming:latest"
