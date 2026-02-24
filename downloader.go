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

// streamMP3 downloads the best audio stream via yt-dlp and pipes it through a
// dedicated ffmpeg process for transcoding. No temporary files are written.
//
// Two-process pipeline:
//
//	yt-dlp  →  raw audio container (webm/opus or m4a/aac)  →  ffmpeg  →  MP3  →  w
//
// Running ffmpeg ourselves (rather than relying on yt-dlp's post-processor)
// guarantees the bitrate and channel settings are applied correctly when
// streaming to stdout, which yt-dlp's internal post-processor does not reliably
// handle with -o -.
//
// Audio is optimised for OpenAI Whisper (25 MB file size limit):
//
//	-ac 1      mono   — Whisper is single-channel; stereo doubles size for no gain
//	-ar 16000  16 kHz — Whisper's native sample rate
//	-b:a 32k   32 kbps CBR — ~2.4 MB per 10 min; 100-min video ≈ 24 MB
func streamMP3(url string, w io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// yt-dlp: select best audio-only format, write raw container to stdout.
	// We skip -x so yt-dlp does not invoke its own ffmpeg post-processor.
	ytdlp := exec.CommandContext(ctx, "yt-dlp",
		"--no-playlist",
		"-f", "bestaudio",
		"--no-warnings",
		"-o", "-",
		url,
	)

	// ffmpeg: read raw audio from stdin, transcode to MP3 with Whisper settings.
	ffmpeg := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", "pipe:0",
		"-ac", "1",
		"-ar", "16000",
		"-b:a", "32k",
		"-f", "mp3",
		"pipe:1",
	)

	// Wire: yt-dlp stdout → ffmpeg stdin → w
	pr, pw := io.Pipe()
	ytdlp.Stdout = pw
	ffmpeg.Stdin = pr
	ffmpeg.Stdout = w

	var ytdlpStderr, ffmpegStderr bytes.Buffer
	ytdlp.Stderr = &ytdlpStderr
	ffmpeg.Stderr = &ffmpegStderr

	if err := ytdlp.Start(); err != nil {
		return fmt.Errorf("could not start yt-dlp: %w", err)
	}
	if err := ffmpeg.Start(); err != nil {
		ytdlp.Process.Kill()
		return fmt.Errorf("could not start ffmpeg: %w", err)
	}

	// Wait for yt-dlp; closing pw with its error (or nil) signals EOF/error to
	// ffmpeg's stdin, allowing ffmpeg to flush and exit cleanly.
	ytdlpErr := ytdlp.Wait()
	pw.CloseWithError(ytdlpErr)

	ffmpegErr := ffmpeg.Wait()

	if ytdlpErr != nil {
		return fmt.Errorf("yt-dlp failed: %w — stderr: %s", ytdlpErr, ytdlpStderr.String())
	}
	if ffmpegErr != nil {
		return fmt.Errorf("ffmpeg failed: %w — stderr: %s", ffmpegErr, ffmpegStderr.String())
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
