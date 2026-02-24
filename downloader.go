package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

// fetchTitle calls yt-dlp to retrieve the video title without downloading.
// Used to set a human-readable filename on the HTTP response.
func fetchTitle(url string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--no-playlist",
		"--print", "%(title)s",
		"--no-warnings",
		url,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("yt-dlp title fetch failed: %w — stderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// streamMP3 invokes yt-dlp to download audio and transcode it to MP3 via
// ffmpeg, streaming the result directly to w. No temporary files are written.
//
// Audio is optimised for OpenAI Whisper:
//   - 32 kbps CBR  — at 32 kbps a 100-minute video fits within Whisper's 25 MB limit
//   - Mono         — Whisper is single-channel; stereo doubles size for no benefit
//   - 16 kHz       — Whisper's native sample rate; higher rates are downsampled anyway
//
// yt-dlp flags used:
//
//	-x                           extract audio only
//	--audio-format mp3           transcode to MP3 via ffmpeg
//	--audio-quality 32K          32 kbps CBR (keeps files well under 25 MB)
//	--postprocessor-args         pass extra ffmpeg flags: mono (-ac 1) + 16 kHz (-ar 16000)
//	-o -                         write output to stdout
//	--no-playlist                never download a playlist, only the single video
func streamMP3(url string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--no-playlist",
		"-x",
		"--audio-format", "mp3",
		"--audio-quality", "32K",
		"--postprocessor-args", "ffmpeg:-ac 1 -ar 16000",
		"--no-warnings",
		"-o", "-",
		url,
	)

	var stderr bytes.Buffer
	cmd.Stdout = w
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("yt-dlp stream failed: %w — stderr: %s", err, stderr.String())
	}

	return nil
}

// ytdlpInfo is a minimal struct for parsing yt-dlp JSON output.
type ytdlpInfo struct {
	Title string `json:"title"`
}

// fetchInfo returns parsed metadata from yt-dlp --dump-json.
// Not used in the hot path but kept here for potential future use.
func fetchInfo(url string) (*ytdlpInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "yt-dlp",
		"--no-playlist",
		"--dump-json",
		"--no-warnings",
		url,
	)

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("yt-dlp info failed: %w — stderr: %s", err, stderr.String())
	}

	var info ytdlpInfo
	if err := json.NewDecoder(&out).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to parse yt-dlp JSON: %w", err)
	}

	return &info, nil
}

// sanitizeFilename strips characters that are unsafe in HTTP Content-Disposition
// filenames and filesystem paths, replacing them with underscores.
func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.' || r == '(' || r == ')' || r == '[' || r == ']':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "audio"
	}
	return s
}
