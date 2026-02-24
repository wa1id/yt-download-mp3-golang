package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const version = "1.0.0"

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
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := streamMP3(req.URL, w); err != nil {
		// Headers already sent, we can't change the status code at this point.
		// Log the error so it's visible in Portainer logs.
		log.Printf("stream error for %s: %v", req.URL, err)
		return
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
