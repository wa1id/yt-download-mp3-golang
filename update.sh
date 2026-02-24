#!/usr/bin/env bash
# update.sh — pull latest code, rebuild image, and restart the container.
# Run this on your home server via SSH whenever you push changes to GitHub.
#
# Usage: bash update.sh [network-name]
#   network-name defaults to "apps-net"

set -euo pipefail

CONTAINER_NAME="yt-downloader"
IMAGE_NAME="yt-download-mp3"
NETWORK="${1:-apps-net}"
PORT="8080"

echo "==> Pulling latest code..."
git pull

echo "==> Building Docker image: $IMAGE_NAME..."
docker build -t "$IMAGE_NAME" .

echo "==> Stopping and removing existing container (if any)..."
docker stop "$CONTAINER_NAME" 2>/dev/null || true
docker rm   "$CONTAINER_NAME" 2>/dev/null || true

echo "==> Starting new container..."
docker run -d \
  --name "$CONTAINER_NAME" \
  --network "$NETWORK" \
  --restart unless-stopped \
  -p "${PORT}:${PORT}" \
  "$IMAGE_NAME"

echo "==> Done. Container status:"
docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
