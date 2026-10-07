package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// AudioRangeHandler serves byte range requests for arbitrary audio streams.
func AudioRangeHandler(audioData []byte) http.HandlerFunc {
	totalLen := int64(len(audioData))

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Type", "audio/mpeg")

		rangeHeader := r.Header.Get("Range")

		// If no Range header, serve full content
		if rangeHeader == "" {
			w.Header().Set("Content-Length", strconv.FormatInt(totalLen, 10))
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(audioData)
			return
		}

		// Parse Range: bytes=start-end
		if !strings.HasPrefix(rangeHeader, "bytes=") {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		spec := strings.TrimPrefix(rangeHeader, "bytes=")
		parts := strings.Split(spec, "-")
		if len(parts) != 2 {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}

		var start, end int64
		var err error

		if parts[0] == "" {
			// Suffix range: bytes=-500
			suffixLen, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || suffixLen <= 0 {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if suffixLen > totalLen {
				suffixLen = totalLen
			}
			start = totalLen - suffixLen
			end = totalLen - 1
		} else {
			start, err = strconv.ParseInt(parts[0], 10, 64)
			if err != nil || start < 0 || start >= totalLen {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			if parts[1] == "" {
				// Open-ended: bytes=500-
				end = totalLen - 1
			} else {
				// Exact range: bytes=500-1000
				end, err = strconv.ParseInt(parts[1], 10, 64)
				if err != nil || end < start {
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				if end >= totalLen {
					end = totalLen - 1
				}
			}
		}

		contentLength := end - start + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalLen))
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
		w.WriteHeader(http.StatusPartialContent)

		if r.Method != http.MethodHead {
			_, _ = io.Copy(w, io.NewSectionReader(strings.NewReader(string(audioData)), start, contentLength))
		}
	}
}

func main() {
	// Generate 1 MiB synthetic audio buffer for demonstration
	synthAudio := make([]byte, 1024*1024)
	for i := range synthAudio {
		synthAudio[i] = byte(i % 256)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/audio", AudioRangeHandler(synthAudio))

	server := &http.Server{
		Addr:         "127.0.0.1:8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	slog.Info("Range audio prototype listening on http://127.0.0.1:8080/audio")
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("HTTP server failed", "error", err)
		os.Exit(1)
	}
}
