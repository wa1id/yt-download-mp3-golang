# ─── Stage 1: Build the Go binary ────────────────────────────────────────────
FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o yt-download-mp3 .


# ─── Stage 2: Runtime image ───────────────────────────────────────────────────
# debian:bookworm-slim gives us apt for yt-dlp + ffmpeg without excess bloat
FROM debian:bookworm-slim

# Install ffmpeg and curl (needed to download yt-dlp binary)
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    curl \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Download the yt-dlp standalone binary (no Python required)
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp \
    -o /usr/local/bin/yt-dlp \
    && chmod +x /usr/local/bin/yt-dlp

# Copy compiled Go binary from builder stage
COPY --from=builder /app/yt-download-mp3 /usr/local/bin/yt-download-mp3

# Run as non-root user for security
RUN useradd -r -s /bin/false appuser
USER appuser

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/yt-download-mp3"]
