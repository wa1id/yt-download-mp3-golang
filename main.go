package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

const version = "1.1.1"

type errorResponse struct {
	Error string `json:"error"`
	URL   string `json:"url,omitempty"`
}

func writeError(w http.ResponseWriter, status int, msg string, url string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorResponse{Error: msg, URL: url})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": version,
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed, use POST", "")
		return
	}

	var req struct {
		URL string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body", "")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing required field: url", "")
		return
	}

	log.Printf("download request: %s", req.URL)

	title, err := fetchTitle(req.URL)
	if err != nil {
		log.Printf("could not fetch title for %s: %v (continuing with fallback name)", req.URL, err)
		title = "audio"
	}

	filename := sanitizeFilename(title) + ".mp3"

	// Start the stream in a background goroutine writing into a pipe.
	// We read the first chunk before committing HTTP headers — this lets us
	// return a proper JSON error if the download fails before producing any audio,
	// instead of sending a 200 OK with an empty body.
	pr, pw := io.Pipe()
	errCh := make(chan error, 1)
	go func() {
		err := streamMP3(req.URL, pw)
		pw.CloseWithError(err)
		errCh <- err
	}()

	// Peek: try to read the first chunk from the pipe.
	firstChunk := make([]byte, 4096)
	n, _ := pr.Read(firstChunk)

	if n == 0 {
		// No bytes arrived — the download failed before producing any audio.
		// Drain the error from the goroutine and surface it to the caller.
		pr.Close()
		streamErr := <-errCh
		msg := "download produced no output"
		if streamErr != nil {
			msg = streamErr.Error()
		}
		log.Printf("download failed for %s: %v", req.URL, streamErr)
		writeError(w, http.StatusInternalServerError, msg, req.URL)
		return
	}

	// Data is flowing — safe to commit response headers now.
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// Write the already-buffered chunk, then stream the rest.
	if _, err := w.Write(firstChunk[:n]); err != nil {
		log.Printf("write error for %s: %v", req.URL, err)
		return
	}
	if _, err := io.Copy(w, pr); err != nil {
		log.Printf("stream copy error for %s: %v", req.URL, err)
	}

	log.Printf("download complete: %s", filename)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/download", handleDownload)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10 * time.Minute, // long timeout: large files can take a while to stream
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("yt-download-mp3 v%s listening on port %s", version, port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
