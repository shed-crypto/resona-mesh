package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/p2p"
	"resonamesh/internal/player"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
	"resonamesh/web"
)

const AppVersion = "0.1.0-alpha"

// Server provides the local HTTP REST and audio streaming API for ResonaMesh.
type Server struct {
	cfg          *config.Config
	catalog      *catalog.Catalog
	importer     *importer.Importer
	streamer     *player.Streamer
	peerTracker  *p2p.PeerTracker
	publisherKey ed25519.PrivateKey
	startTime    time.Time
	mux          *http.ServeMux
	handler      http.Handler
	httpServer   *http.Server
}

// NewServer initializes and wires routes and middlewares for the HTTP API.
func NewServer(
	cfg *config.Config,
	cat *catalog.Catalog,
	imp *importer.Importer,
	str *player.Streamer,
	privKey ed25519.PrivateKey,
) *Server {
	s := &Server{
		cfg:          cfg,
		catalog:      cat,
		importer:     imp,
		streamer:     str,
		publisherKey: privKey,
		startTime:    time.Now(),
		mux:          http.NewServeMux(),
	}

	s.registerRoutes()

	// Wrap mux with CORS and recovery middlewares
	s.handler = corsMiddleware(recoveryMiddleware(s.mux))

	s.httpServer = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      s.handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	return s
}

// Handler returns the configured http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.handler
}

// Start listens and serves HTTP requests.
func (s *Server) Start() error {
	slog.Info("Starting HTTP REST API server", "addr", s.cfg.HTTPAddr)
	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("api: http server error: %w", err)
	}
	return nil
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// SetPeerTracker binds the P2P peer tracker for peer telemetry endpoints.
func (s *Server) SetPeerTracker(pt *p2p.PeerTracker) {
	s.peerTracker = pt
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/v1/tracks", s.handleListTracks)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}", s.handleGetTrack)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}/manifest", s.handleGetManifest)
	s.mux.HandleFunc("POST /api/v1/tracks/{id}/pin", s.handlePinTrack)
	s.mux.HandleFunc("DELETE /api/v1/tracks/{id}", s.handleDeleteTrack)
	s.mux.HandleFunc("POST /api/v1/tracks/import", s.handleImportTrack)
	s.mux.HandleFunc("GET /api/v1/stream/{id}", s.handleStream)
	s.mux.HandleFunc("GET /api/v1/peers", s.handleListPeers)
	s.mux.HandleFunc("GET /api/v1/node/status", s.handleNodeStatus)

	if webHandler, err := web.DistHandler(); err == nil {
		s.mux.Handle("/", webHandler)
	}
}

// handleListTracks returns paginated tracks with optional search and pin filters.
func (s *Server) handleListTracks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	pinnedVal := r.URL.Query().Get("pinned")
	pinnedOnly := pinnedVal == "1" || strings.EqualFold(pinnedVal, "true")

	tracks, total, err := s.catalog.ListTracks(r.Context(), catalog.Query{
		Search:     q,
		Limit:      limit,
		Offset:     offset,
		PinnedOnly: pinnedOnly,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query tracks")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":  tracks,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// handleGetTrack returns full metadata and availability for a single track.
func (s *Server) handleGetTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	track, err := s.catalog.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve track")
		return
	}

	writeJSON(w, http.StatusOK, track)
}

// handleGetManifest returns the signed Manifest for a track.
func (s *Server) handleGetManifest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	manifest, err := s.catalog.GetManifest(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "manifest not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve manifest")
		return
	}

	writeJSON(w, http.StatusOK, manifest)
}

// handlePinTrack toggles or sets the local pinning status of a track.
func (s *Server) handlePinTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := s.catalog.SetPinned(r.Context(), id, req.Pinned); err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update pin status")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content_id": id,
		"pinned":     req.Pinned,
	})
}

// handleDeleteTrack removes a track from the catalog.
func (s *Server) handleDeleteTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	if err := s.catalog.DeleteTrack(r.Context(), id); err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to delete track")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "deleted",
		"content_id": id,
	})
}

// handleImportTrack handles multipart/form-data upload of an audio file and creates catalog entries.
func (s *Server) handleImportTrack(w http.ResponseWriter, r *http.Request) {
	// 500 MB max request body
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to parse multipart form or file too large")
		return
	}

	file, _, err := r.FormFile("audio_file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "audio_file multipart field is required")
		return
	}
	defer file.Close()

	title := strings.TrimSpace(r.FormValue("title"))
	artist := strings.TrimSpace(r.FormValue("artist"))
	description := strings.TrimSpace(r.FormValue("description"))
	licenseCode := strings.TrimSpace(r.FormValue("license"))
	if licenseCode == "" {
		licenseCode = protocol.LicenseCCBY40
	}

	bitrateKbps := 192
	if b := r.FormValue("bitrate_kbps"); b != "" {
		if parsed, err := strconv.Atoi(b); err == nil && parsed > 0 {
			bitrateKbps = parsed
		}
	}

	var durationMs int64
	if d := r.FormValue("duration_ms"); d != "" {
		if parsed, err := strconv.ParseInt(d, 10, 64); err == nil && parsed >= 0 {
			durationMs = parsed
		}
	}

	manifest, err := s.importer.Import(r.Context(), importer.ImportParams{
		Title:        title,
		Artist:       artist,
		Description:  description,
		DurationMs:   durationMs,
		BitrateKbps:  bitrateKbps,
		LicenseCode:  licenseCode,
		PublisherKey: s.publisherKey,
		AudioData:    file,
	})
	if err != nil {
		if errors.Is(err, importer.ErrTrackAlreadyExists) {
			writeJSONError(w, http.StatusConflict, "track already exists in catalog")
			return
		}
		if errors.Is(err, importer.ErrMissingTitle) ||
			errors.Is(err, importer.ErrMissingArtist) ||
			errors.Is(err, importer.ErrInvalidLicense) ||
			errors.Is(err, importer.ErrNilAudioStream) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("import failed: %v", err))
		return
	}

	track, err := s.catalog.GetTrack(r.Context(), manifest.ContentID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "track imported but failed to load record")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"manifest": manifest,
		"track":    track,
	})
}

// handleStream routes HTTP 206 audio streaming requests to the player.Streamer.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}
	s.streamer.StreamTrack(w, r, id)
}

// handleListPeers returns active P2P swarm peers discovered or connected.
func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	if s.peerTracker == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"peers": []p2p.PeerRecord{},
			"total": 0,
		})
		return
	}

	peers := s.peerTracker.Peers()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"peers": peers,
		"total": len(peers),
	})
}

// handleNodeStatus returns runtime status, storage usage, and node publisher identity.
func (s *Server) handleNodeStatus(w http.ResponseWriter, r *http.Request) {
	totalTracks, storedBytes, err := s.catalog.Stats(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to calculate node statistics")
		return
	}

	pubKey := s.publisherKey.Public().(ed25519.PublicKey)
	pubKeyHex := crypto.PublicKeyToHex(pubKey)

	connectedPeers := 0
	if s.peerTracker != nil {
		for _, p := range s.peerTracker.Peers() {
			if p.Connected {
				connectedPeers++
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":             AppVersion,
		"publisher_key":       pubKeyHex,
		"data_dir":            s.cfg.DataDir,
		"storage_quota_bytes": s.cfg.StorageQuotaBytes,
		"storage_used_bytes":  storedBytes,
		"total_tracks":        totalTracks,
		"connected_peers":     connectedPeers,
		"uptime_seconds":      int64(time.Since(s.startTime).Seconds()),
	})
}

// corsMiddleware injects CORS headers for local frontend integration and audio streaming.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range, Authorization")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges, Content-Length")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware catches handler panics and returns HTTP 500 JSON.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("HTTP panic recovered", "panic", rec, "path", r.URL.Path)
				writeJSONError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{
		"error": message,
	})
}
