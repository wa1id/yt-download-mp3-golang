#!/bin/sh
set -e

echo "Checking for yt-dlp updates..."
yt-dlp -U 2>&1 || echo "Warning: yt-dlp update check failed, continuing with installed version"

echo "Starting yt-download-mp3 v$(yt-dlp --version 2>/dev/null || echo unknown)..."
exec /usr/local/bin/yt-download-mp3
