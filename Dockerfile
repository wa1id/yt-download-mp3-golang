# ─── Stage 1: Build the Go binary ────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o yt-download-mp3 .


# ─── Stage 2: Runtime image ───────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install ffmpeg and curl
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user with a home directory
RUN useradd -r -s /bin/false -m appuser

# Install yt-dlp into a directory owned by appuser so the auto-update
# on container start can overwrite the binary without needing root.
RUN mkdir -p /home/appuser/.local/bin && \
    curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_linux \
    -o /home/appuser/.local/bin/yt-dlp \
    && chmod +x /home/appuser/.local/bin/yt-dlp \
    && chown -R appuser:appuser /home/appuser/.local

# Copy compiled Go binary and entrypoint
COPY --from=builder /app/yt-download-mp3 /usr/local/bin/yt-download-mp3
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER appuser

# yt-dlp lives in appuser's local bin
ENV PATH="/home/appuser/.local/bin:$PATH"

EXPOSE 8080

ENTRYPOINT ["/entrypoint.sh"]
