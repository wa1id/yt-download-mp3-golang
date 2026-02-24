# yt-download-mp3

A lightweight Go HTTP service that downloads a YouTube video and streams it back as an MP3. Built to replace external YouTube-to-MP3 APIs in self-hosted n8n workflows.

Internally wraps [yt-dlp](https://github.com/yt-dlp/yt-dlp) + ffmpeg — no temp files, output is streamed directly to the HTTP response.

---

## API

### `POST /download`

Download a YouTube URL as MP3.

**Request**
```json
{ "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ" }
```

**Response**
- `200 OK` — `audio/mpeg` binary stream with `Content-Disposition: attachment; filename="<title>.mp3"`
- `400 Bad Request` — `{"error": "..."}`
- `500 Internal Server Error` — `{"error": "..."}`

### `GET /health`

```json
{ "status": "ok", "version": "1.0.0" }
```

---

## Server setup (first time)

SSH into your home server, then:

```bash
# 1. Install Docker if not already installed (skip if using Portainer)
#    https://docs.docker.com/engine/install/

# 2. Create a shared Docker network so n8n and this service can talk
docker network create apps-net

# 3. Attach your existing n8n container to the network
docker network connect apps-net n8n   # replace "n8n" with your container name

# 4. Clone this repo
git clone https://github.com/wa1id/yt-download-mp3-golang.git
cd yt-download-mp3-golang

# 5. Build and run
bash update.sh
```

The container starts on port `8080` and will restart automatically unless stopped.

---

## Updating after a code change

```bash
cd yt-download-mp3-golang
bash update.sh
```

That's it — pulls latest code, rebuilds the image, and replaces the running container.

---

## n8n integration

Add an **HTTP Request** node to your workflow:

| Field | Value |
|---|---|
| Method | `POST` |
| URL | `http://yt-downloader:8080/download` |
| Body Content Type | `JSON` |
| Body | `{ "url": "{{ $json.youtube_url }}" }` |
| Response Format | `File` |

The node outputs a binary item (`$binary.data`) with `mimeType: audio/mpeg` ready for the next node (e.g. save to disk, upload to S3, send via Telegram, etc.).

> **Note:** `yt-downloader` resolves by container name because both containers are on the same `apps-net` Docker network. No IP addresses needed.

---

## Uptime Kuma

Add an **HTTP(s)** monitor:
- URL: `http://yt-downloader:8080/health` (if Kuma is on `apps-net`)
- or `http://localhost:8080/health` (if using host network)

---

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Port the HTTP server listens on |

---

## How it works

1. `POST /download` receives a YouTube URL
2. `yt-dlp --print %(title)s` fetches the video title (for the filename)
3. `yt-dlp -x --audio-format mp3 --audio-quality 0 -o -` downloads and transcodes audio, writing to stdout
4. The Go server pipes that stdout directly into the HTTP response body — no disk writes

### Why yt-dlp instead of a Go library?

YouTube protects stream URLs with a JavaScript-based signature cipher that changes every few days. yt-dlp has a large community patching breakage within hours. Pure Go libraries are slower to update and more fragile in production.
