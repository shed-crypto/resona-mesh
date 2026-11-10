package api

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/moderation"
	"resonamesh/internal/p2p"
	"resonamesh/internal/player"
	"resonamesh/internal/storage"
	"resonamesh/internal/updater"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
	"resonamesh/web"
)

const AppVersion = "0.1.0-alpha"

// Syncer coordinates manifest synchronization with connected swarm peers.
type Syncer interface {
	SyncFromPeer(ctx context.Context, peerID peer.ID) (int, error)
}

// Server provides the local HTTP REST and audio streaming API for ResonaMesh.
type Server struct {
	cfg          *config.Config
	catalog      *catalog.Catalog
	importer     *importer.Importer
	streamer     *player.Streamer
	peerTracker  *p2p.PeerTracker
	syncer       Syncer
	quotaManager     *storage.QuotaManager
	p2pHost          *p2p.Host
	moderationEngine *moderation.Engine
	moderatorKey     ed25519.PrivateKey
	updaterEngine    *updater.Engine
	releaseHandler   *p2p.ReleaseHandler
	reputationTracker *p2p.PeerReputationTracker
	publisherKey     ed25519.PrivateKey
	startTime        time.Time
	mux              *http.ServeMux
	handler          http.Handler
	httpServer       *http.Server
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

// SetSyncer binds the P2P catalog syncer for swarm synchronization.
func (s *Server) SetSyncer(sy Syncer) {
	s.syncer = sy
}

// SetQuotaManager binds the storage quota manager to expose disk metrics.
func (s *Server) SetQuotaManager(qm *storage.QuotaManager) {
	s.quotaManager = qm
}

// SetP2PHost binds the libp2p host instance for manual peer dialing.
func (s *Server) SetP2PHost(h *p2p.Host) {
	s.p2pHost = h
}

// SetModerationEngine binds the moderation policy engine and optional local moderator private key.
func (s *Server) SetModerationEngine(eng *moderation.Engine, modKey ed25519.PrivateKey) {
	s.moderationEngine = eng
	s.moderatorKey = modKey
}

// SetUpdater binds the auto-updater engine for client upgrade management.
func (s *Server) SetUpdater(u *updater.Engine) {
	s.updaterEngine = u
}

// SetReleaseHandler binds the P2P release handler for swarm release propagation.
func (s *Server) SetReleaseHandler(rh *p2p.ReleaseHandler) {
	s.releaseHandler = rh
}

// SetReputationTracker binds the P2P peer reputation tracker.
func (s *Server) SetReputationTracker(rt *p2p.PeerReputationTracker) {
	s.reputationTracker = rt
}


func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/v1/tracks", s.handleListTracks)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}", s.handleGetTrack)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}/manifest", s.handleGetManifest)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}/cover", s.handleGetTrackCover)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}/download", s.handleDownloadTrack)
	s.mux.HandleFunc("HEAD /api/v1/tracks/{id}/download", s.handleDownloadTrack)
	s.mux.HandleFunc("POST /api/v1/tracks/{id}/pin", s.handlePinTrack)
	s.mux.HandleFunc("POST /api/v1/tracks/{id}/unpin", s.handleUnpinTrack)
	s.mux.HandleFunc("PUT /api/v1/tracks/{id}/lyrics", s.handleUpdateTrackLyrics)
	s.mux.HandleFunc("GET /api/v1/tracks/{id}/recommendations", s.handleTrackRecommendations)
	s.mux.HandleFunc("PUT /api/v1/tracks/{id}/metadata", s.handleUpdateTrackMetadata)
	s.mux.HandleFunc("DELETE /api/v1/tracks/{id}", s.handleDeleteTrack)
	s.mux.HandleFunc("POST /api/v1/tracks/import", s.handleImportTrack)
	s.mux.HandleFunc("GET /api/v1/stream/{id}", s.handleStream)
	s.mux.HandleFunc("HEAD /api/v1/stream/{id}", s.handleStream)
	s.mux.HandleFunc("GET /api/v1/audio/{id}", s.handleStream)
	s.mux.HandleFunc("HEAD /api/v1/audio/{id}", s.handleStream)
	s.mux.HandleFunc("GET /api/v1/peers", s.handleListPeers)
	s.mux.HandleFunc("POST /api/v1/peers/connect", s.handleConnectPeer)
	s.mux.HandleFunc("POST /api/v1/peers/sync", s.handleSyncPeers)
	s.mux.HandleFunc("GET /api/v1/network/reputation", s.handleGetReputation)
	s.mux.HandleFunc("GET /api/v1/storage", s.handleGetStorage)
	s.mux.HandleFunc("GET /api/v1/settings", s.handleGetSettings)
	s.mux.HandleFunc("PUT /api/v1/settings", s.handleUpdateSettings)
	s.mux.HandleFunc("GET /api/v1/node/status", s.handleNodeStatus)
	s.mux.HandleFunc("GET /api/v1/seeds", s.handleSeeds)
	s.mux.HandleFunc("POST /api/v1/reports", s.handleSubmitReport)
	s.mux.HandleFunc("GET /api/v1/reports", s.handleListReports)
	s.mux.HandleFunc("GET /api/v1/moderation/decisions", s.handleListDecisions)
	s.mux.HandleFunc("POST /api/v1/moderation/decisions", s.handleCreateDecision)
	s.mux.HandleFunc("GET /api/v1/moderation/blocked", s.handleListBlocked)
	s.mux.HandleFunc("GET /api/v1/moderation/moderators", s.handleListModerators)
	s.mux.HandleFunc("POST /api/v1/moderation/moderators", s.handleDelegateModerator)
	s.mux.HandleFunc("POST /api/v1/moderation/key-rotations", s.handleApplyKeyRotation)
	s.mux.HandleFunc("GET /api/v1/updater/status", s.handleUpdaterStatus)
	s.mux.HandleFunc("POST /api/v1/updater/check", s.handleUpdaterCheck)
	s.mux.HandleFunc("POST /api/v1/updater/download", s.handleUpdaterDownload)
	s.mux.HandleFunc("POST /api/v1/updater/apply", s.handleUpdaterApply)
	s.mux.HandleFunc("POST /api/v1/updater/rollback", s.handleUpdaterRollback)

	s.mux.HandleFunc("POST /api/v1/identity/export", s.handleExportIdentity)
	s.mux.HandleFunc("POST /api/v1/identity/import", s.handleImportIdentity)

	if s.cfg.Headless {
		s.mux.HandleFunc("GET /", s.handleHeadlessStatusPage)
	} else if webHandler, err := web.DistHandler(); err == nil {
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

	incBlockedVal := r.URL.Query().Get("include_blocked")
	includeBlocked := incBlockedVal == "1" || strings.EqualFold(incBlockedVal, "true")

	tracks, total, err := s.catalog.ListTracks(r.Context(), catalog.Query{
		Search:         q,
		Limit:          limit,
		Offset:         offset,
		PinnedOnly:     pinnedOnly,
		IncludeBlocked: includeBlocked,
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

// handleGetTrackCover streams the binary cover art image for a track.
func (s *Server) handleGetTrackCover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	data, mimeType, err := s.catalog.GetCover(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "cover art not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to retrieve cover art")
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// handlePinTrack toggles or sets the local pinning status of a track with storage quota validation.
func (s *Server) handlePinTrack(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
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

	track, err := s.catalog.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to query track")
		return
	}

	if req.Pinned && s.quotaManager != nil && track.AvailabilityPct < 100.0 {
		missingBytes := int64((100.0 - track.AvailabilityPct) / 100.0 * float64(track.TotalSizeBytes))
		if missingBytes <= 0 {
			missingBytes = int64(protocol.BlockSize)
		}
		tok, err := s.quotaManager.Reserve(r.Context(), missingBytes)
		if err != nil {
			writeJSONError(w, http.StatusConflict, "insufficient storage quota to pin track")
			return
		}
		s.quotaManager.Release(tok)
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

// handleUnpinTrack unpins a track, permitting its cached blocks to be evicted under storage pressure.
func (s *Server) handleUnpinTrack(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	if err := s.catalog.SetPinned(r.Context(), id, false); err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to unpin track")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content_id": id,
		"pinned":     false,
	})
}

// handleUpdateTrackLyrics updates synchronized lyrics for a track.
func (s *Server) handleUpdateTrackLyrics(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	var req struct {
		Lyrics string `json:"lyrics"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json request body: "+err.Error())
		return
	}

	if err := s.catalog.UpdateTrackLyrics(r.Context(), id, req.Lyrics); err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update track lyrics")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "updated",
		"content_id": id,
		"lyrics":     req.Lyrics,
	})
}



// handleTrackRecommendations returns top recommended tracks for a given seed track.
func (s *Server) handleTrackRecommendations(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	recs, err := s.catalog.GetRecommendations(r.Context(), id, limit)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to get recommendations: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"content_id":      id,
		"recommendations": recs,
	})
}

// handleUpdateTrackMetadata updates genre, tags, bpm, and mood for an existing track.
func (s *Server) handleUpdateTrackMetadata(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	var req struct {
		Genre string   `json:"genre"`
		Tags  []string `json:"tags"`
		BPM   int      `json:"bpm"`
		Mood  string   `json:"mood"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json request body: "+err.Error())
		return
	}

	if err := s.catalog.UpdateTrackMetadata(r.Context(), id, req.Genre, req.Tags, req.BPM, req.Mood); err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to update track metadata: "+err.Error())
		return
	}

	updated, err := s.catalog.GetTrack(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to load updated track")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "updated",
		"track":  updated,
	})
}
// handleDeleteTrack removes a track from the catalog.
func (s *Server) handleDeleteTrack(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
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
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
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
	lyrics := strings.TrimSpace(r.FormValue("lyrics"))
	genre := strings.TrimSpace(r.FormValue("genre"))
	mood := strings.TrimSpace(r.FormValue("mood"))
	bpm := 0
	if b := r.FormValue("bpm"); b != "" {
		if parsed, err := strconv.Atoi(b); err == nil && parsed >= 0 {
			bpm = parsed
		}
	}
	var tags []string
	if rawTags := r.FormValue("tags"); rawTags != "" {
		if strings.HasPrefix(strings.TrimSpace(rawTags), "[") {
			_ = json.Unmarshal([]byte(rawTags), &tags)
		} else {
			for _, part := range strings.Split(rawTags, ",") {
				if t := strings.TrimSpace(part); t != "" {
					tags = append(tags, t)
				}
			}
		}
	}
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

	var coverBytes []byte
	var coverMime string
	if coverFile, coverHeader, err := r.FormFile("cover_file"); err == nil && coverFile != nil {
		defer coverFile.Close()
		// Limit cover art file to 5MB
		data, readErr := io.ReadAll(io.LimitReader(coverFile, 5*1024*1024))
		if readErr == nil && len(data) > 0 {
			coverBytes = data
			coverMime = coverHeader.Header.Get("Content-Type")
			if coverMime == "" {
				coverMime = http.DetectContentType(data)
			}
		}
	}

	manifest, err := s.importer.Import(r.Context(), importer.ImportParams{
		Title:        title,
		Artist:       artist,
		Description:  description,
		Lyrics:       lyrics,
		Genre:        genre,
		Tags:         tags,
		BPM:          bpm,
		Mood:         mood,
		DurationMs:   durationMs,
		BitrateKbps:  bitrateKbps,
		LicenseCode:  licenseCode,
		PublisherKey: s.publisherKey,
		AudioData:    file,
		CoverData:    coverBytes,
		CoverMime:    coverMime,
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
		slog.Warn("Audio stream request missing track id", "url", r.URL.String())
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}
	slog.Info("Audio stream requested", "content_id", id, "method", r.Method, "range", r.Header.Get("Range"))
	s.streamer.StreamTrack(w, r, id)
}

// handleDownloadTrack streams the complete audio track with Content-Disposition attachment header for direct file export.
func (s *Server) handleDownloadTrack(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		slog.Warn("Track download request missing track id", "url", r.URL.String())
		writeJSONError(w, http.StatusBadRequest, "track id is required")
		return
	}

	track, err := s.catalog.GetTrack(r.Context(), id)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			writeJSONError(w, http.StatusNotFound, "track not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "failed to get track")
		return
	}

	var filename string
	artist := strings.TrimSpace(track.Artist)
	title := strings.TrimSpace(track.Title)
	if artist != "" && title != "" {
		filename = fmt.Sprintf("%s - %s.mp3", artist, title)
	} else if title != "" {
		filename = fmt.Sprintf("%s.mp3", title)
	} else if artist != "" {
		filename = fmt.Sprintf("%s.mp3", artist)
	} else {
		filename = fmt.Sprintf("%s.mp3", id)
	}
	filename = SanitizeFilename(filename)

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q; filename*=UTF-8''%s", filename, url.PathEscape(filename)))
	s.streamer.StreamTrack(w, r, id)
}

// SanitizeFilename cleans audio track file names for safe Content-Disposition headers.
func SanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			b.WriteRune('_')
		default:
			if r < 32 {
				b.WriteRune('_')
			} else {
				b.WriteRune(r)
			}
		}
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" || cleaned == ".mp3" {
		return "track.mp3"
	}
	return cleaned
}

// EnrichedPeerRecord wraps runtime peer discovery state with empirical reputation metrics and transport classification.
type EnrichedPeerRecord struct {
	p2p.PeerRecord
	ConnectionType   string  `json:"connection_type"`
	Score            float64 `json:"reputation_score"`
	BlocksVerified   int64   `json:"blocks_verified"`
	HashMismatches   int64   `json:"hash_mismatches"`
	StreamErrors     int64   `json:"stream_errors"`
	AvgLatencyMs     float64 `json:"avg_latency_ms"`
	IsQuarantined    bool    `json:"is_quarantined"`
	QuarantinedUntil string  `json:"quarantined_until,omitempty"`
}

// handleListPeers returns active P2P swarm peers discovered or connected, enriched with reputation state.
func (s *Server) handleListPeers(w http.ResponseWriter, r *http.Request) {
	if s.peerTracker == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"peers":       []EnrichedPeerRecord{},
			"total":       0,
			"cache_limit": p2p.MaxTrackedPeers,
		})
		return
	}

	rawPeers := s.peerTracker.Peers()
	now := time.Now()
	enriched := make([]EnrichedPeerRecord, 0, len(rawPeers))
	for _, p := range rawPeers {
		connType := "known"
		if p.Connected {
			if s.p2pHost != nil {
				if s.p2pHost.HasDirectConnection(p.ID) {
					connType = "direct"
				} else {
					connType = "relayed"
				}
			} else {
				connType = "direct"
				for _, addr := range p.Addrs {
					if strings.Contains(addr, "p2p-circuit") {
						connType = "relayed"
						break
					}
				}
			}
		}

		ep := EnrichedPeerRecord{
			PeerRecord:     p,
			ConnectionType: connType,
			Score:          100.0,
		}
		if s.reputationTracker != nil {
			ep.Score = s.reputationTracker.Score(p.ID)
			ep.IsQuarantined = s.reputationTracker.IsQuarantined(p.ID, now)
			if stats, ok := s.reputationTracker.GetStats(p.ID); ok {
				ep.BlocksVerified = stats.BlocksVerified
				ep.HashMismatches = stats.HashMismatchCount
				ep.StreamErrors = stats.StreamErrors
				ep.AvgLatencyMs = stats.AvgLatencyMs
				if ep.IsQuarantined {
					ep.QuarantinedUntil = stats.QuarantinedUntil.Format(time.RFC3339)
				}
			}
		}
		enriched = append(enriched, ep)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"peers":       enriched,
		"total":       len(enriched),
		"cache_limit": p2p.MaxTrackedPeers,
	})
}

// handleGetReputation returns reputation and quarantine telemetry across all peers.
func (s *Server) handleGetReputation(w http.ResponseWriter, r *http.Request) {
	if s.reputationTracker == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"peers": []interface{}{},
			"total": 0,
		})
		return
	}

	stats := s.reputationTracker.AllStats()
	now := time.Now()
	type peerRepEntry struct {
		p2p.PeerTransferStats
		Score         float64 `json:"score"`
		IsQuarantined bool    `json:"is_quarantined"`
	}

	entries := make([]peerRepEntry, 0, len(stats))
	for _, st := range stats {
		entries = append(entries, peerRepEntry{
			PeerTransferStats: st,
			Score:             s.reputationTracker.Score(st.PeerID),
			IsQuarantined:     s.reputationTracker.IsQuarantined(st.PeerID, now),
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"peers": entries,
		"total": len(entries),
	})
}

// handleSyncPeers triggers catalog synchronization across all connected swarm peers.
func (s *Server) handleSyncPeers(w http.ResponseWriter, r *http.Request) {
	if s.syncer == nil || s.peerTracker == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":          "syncer_not_configured",
			"imported_tracks": 0,
		})
		return
	}

	peers := s.peerTracker.Peers()
	totalImported := 0
	ctx := r.Context()

	for _, p := range peers {
		if !p.Connected {
			continue
		}
		count, err := s.syncer.SyncFromPeer(ctx, p.ID)
		if err == nil {
			totalImported += count
		} else {
			slog.Debug("Manual peer sync failed", "peer_id", p.ID.String(), "err", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":          "success",
		"imported_tracks": totalImported,
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
	directPeers := 0
	relayConnected := false
	if s.p2pHost != nil {
		activePeers := s.p2pHost.ConnectedPeers()
		connectedPeers = len(activePeers)
		for _, pID := range activePeers {
			isWS := false
			for _, conn := range s.p2pHost.Raw().Network().ConnsToPeer(pID) {
				mStr := conn.RemoteMultiaddr().String()
				if strings.Contains(mStr, "/ws") || strings.Contains(mStr, "/wss") {
					isWS = true
					break
				}
			}
			if isWS {
				relayConnected = true
			} else {
				directPeers++
			}
		}
	} else if s.peerTracker != nil {
		for _, p := range s.peerTracker.Peers() {
			if p.Connected {
				connectedPeers++
			}
		}
	}

	var p2pPeerID string
	listenAddrs := []string{}
	if s.p2pHost != nil {
		p2pPeerID = s.p2pHost.ID().String()
		for _, ma := range s.p2pHost.Addrs() {
			listenAddrs = append(listenAddrs, fmt.Sprintf("%s/p2p/%s", ma.String(), p2pPeerID))
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
		"direct_peers":        directPeers,
		"relay_connected":     relayConnected,
		"bootstrap_seeds":     s.cfg.BootstrapSeeds,
		"uptime_seconds":      int64(time.Since(s.startTime).Seconds()),
		"peer_id":             p2pPeerID,
		"listen_addrs":        listenAddrs,
		"is_moderator":        s.moderatorKey != nil,
	})
}

// handleSeeds returns an array of public multiaddresses for this node as a dynamic bootstrap endpoint.
func (s *Server) handleSeeds(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Content-Type", "application/json")

	if s.p2pHost == nil {
		_ = json.NewEncoder(w).Encode([]string{})
		return
	}

	hostHeader := r.Host
	hostname, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		hostname = hostHeader
	}

	peerID := s.p2pHost.ID().String()
	wanAddr := fmt.Sprintf("/dns4/%s/tcp/443/wss/p2p/%s", hostname, peerID)
	_ = json.NewEncoder(w).Encode([]string{wanAddr})
}

// handleConnectPeer connects to a remote peer via an explicitly supplied multiaddress string.
func (s *Server) handleConnectPeer(w http.ResponseWriter, r *http.Request) {
	if s.p2pHost == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "p2p host not initialized")
		return
	}

	var req struct {
		Addr string `json:"addr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Addr) == "" {
		writeJSONError(w, http.StatusBadRequest, "peer multiaddr is required")
		return
	}

	addrInfo, err := peer.AddrInfoFromString(strings.TrimSpace(req.Addr))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid multiaddr: %v", err))
		return
	}

	dialCtx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := s.p2pHost.Connect(dialCtx, *addrInfo); err != nil {
		writeJSONError(w, http.StatusBadGateway, fmt.Sprintf("failed to connect to peer: %v", err))
		return
	}

	slog.Info("Successfully connected to peer via manual multiaddr", "peer_id", addrInfo.ID.String())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "connected",
		"peer_id": addrInfo.ID.String(),
	})
}

// handleGetStorage returns detailed storage quota utilization metrics.
func (s *Server) handleGetStorage(w http.ResponseWriter, r *http.Request) {
	if s.quotaManager == nil {
		totalTracks, storedBytes, _ := s.catalog.Stats(r.Context())
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"quota_bytes":     s.cfg.StorageQuotaBytes,
			"stored_bytes":    storedBytes,
			"reserved_bytes":  0,
			"available_bytes": s.cfg.StorageQuotaBytes - storedBytes,
			"total_tracks":    totalTracks,
		})
		return
	}

	usage, err := s.quotaManager.Usage(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to query storage quota")
		return
	}

	writeJSON(w, http.StatusOK, usage)
}

// handleGetSettings returns configurable node daemon parameters.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storage_quota_bytes": s.cfg.StorageQuotaBytes,
		"keeper_enabled":      s.cfg.KeeperEnabled,
		"p2p_port":            s.cfg.P2PPort,
		"http_addr":           s.cfg.HTTPAddr,
	})
}

// handleUpdateSettings dynamically adjusts configurable node daemon settings.
func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "headless relay mode active: mutations are disabled")
		return
	}
	var req struct {
		StorageQuotaBytes *int64 `json:"storage_quota_bytes"`
		KeeperEnabled     *bool  `json:"keeper_enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if req.StorageQuotaBytes != nil {
		if *req.StorageQuotaBytes <= 0 {
			writeJSONError(w, http.StatusBadRequest, "storage quota must be greater than zero")
			return
		}
		s.cfg.StorageQuotaBytes = *req.StorageQuotaBytes
		if s.quotaManager != nil {
			s.quotaManager.SetQuotaBytes(*req.StorageQuotaBytes)
		}
	}

	if req.KeeperEnabled != nil {
		s.cfg.KeeperEnabled = *req.KeeperEnabled
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"storage_quota_bytes": s.cfg.StorageQuotaBytes,
		"keeper_enabled":      s.cfg.KeeperEnabled,
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

// handleSubmitReport handles submission of abuse or copyright reports.
func (s *Server) handleSubmitReport(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	var req struct {
		ContentID         string `json:"content_id"`
		PayloadHash       string `json:"payload_hash"`
		ReasonCode        string `json:"reason_code"`
		Description       string `json:"description"`
		EvidenceReference string `json:"evidence_reference"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json request body")
		return
	}

	report, err := s.moderationEngine.SubmitReport(r.Context(), moderation.CreateReportParams{
		ContentID:         req.ContentID,
		PayloadHash:       req.PayloadHash,
		ReasonCode:        req.ReasonCode,
		Description:       req.Description,
		EvidenceReference: req.EvidenceReference,
	})
	if err != nil {
		if errors.Is(err, moderation.ErrMissingContentID) || errors.Is(err, moderation.ErrInvalidReasonCode) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, report)
}

// handleListReports lists outbox and received reports.
func (s *Server) handleListReports(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	reports, err := s.moderationEngine.ListReports(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if reports == nil {
		reports = []moderation.ReportRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": reports,
		"total": len(reports),
	})
}

// handleListDecisions lists moderation decisions recorded on this node.
func (s *Server) handleListDecisions(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	decisions, err := s.moderationEngine.ListDecisions(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if decisions == nil {
		decisions = []moderation.DecisionRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": decisions,
		"total": len(decisions),
	})
}

// handleCreateDecision applies or creates a moderation decision.
func (s *Server) handleCreateDecision(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	var raw map[string]interface{}
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to read request body")
		return
	}

	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	// If signature is present, it's a pre-signed DecisionEvent to apply
	if sig, ok := raw["signature"].(string); ok && sig != "" {
		var dec protocol.DecisionEvent
		if err := json.Unmarshal(bodyBytes, &dec); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid decision event structure")
			return
		}

		if err := s.moderationEngine.ApplyDecision(r.Context(), &dec); err != nil {
			if errors.Is(err, moderation.ErrUntrustedModerator) || errors.Is(err, moderation.ErrInvalidSignature) || errors.Is(err, moderation.ErrForkDetected) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":   "applied",
			"decision": dec,
		})
		return
	}

	// Otherwise, if this node holds moderator private key, create and sign a new decision
	if s.moderatorKey == nil {
		writeJSONError(w, http.StatusForbidden, "node is not configured with a moderator private key to sign decisions")
		return
	}

	var params moderation.CreateDecisionParams
	if err := json.Unmarshal(bodyBytes, &params); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid create decision params")
		return
	}

	decision, err := s.moderationEngine.CreateDecision(r.Context(), s.moderatorKey, params)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, decision)
}

// handleListBlocked returns all content currently blocked by moderation policy.
func (s *Server) handleListBlocked(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	blocked, err := s.moderationEngine.ListBlocked(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if blocked == nil {
		blocked = []moderation.BlockedRecord{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": blocked,
		"total": len(blocked),
	})
}

type ExportIdentityRequest struct {
	Passphrase string `json:"passphrase"`
}

type ImportIdentityRequest struct {
	Bundle     json.RawMessage `json:"bundle"`
	Passphrase string          `json:"passphrase"`
}

type DelegateModeratorRequest struct {
	Action          string `json:"action"` // "APPROVE_MODERATOR" or "REVOKE_MODERATOR"
	ModeratorPubKey string `json:"moderator_pubkey"`
	Alias           string `json:"alias"`
	DurationSec     int64  `json:"duration_sec"`
}

// handleListModerators returns all active and historical moderator certificates.
func (s *Server) handleListModerators(w http.ResponseWriter, r *http.Request) {
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	certs, err := s.moderationEngine.ListModeratorCertificates(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if certs == nil {
		certs = []protocol.ModeratorCertificate{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items": certs,
		"total": len(certs),
	})
}

// handleDelegateModerator delegates or revokes moderator privileges.
func (s *Server) handleDelegateModerator(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "moderator delegation is disabled in headless mode")
		return
	}
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	var req DelegateModeratorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Action != "APPROVE_MODERATOR" && req.Action != "REVOKE_MODERATOR" {
		writeJSONError(w, http.StatusBadRequest, "action must be APPROVE_MODERATOR or REVOKE_MODERATOR")
		return
	}
	if req.ModeratorPubKey == "" {
		writeJSONError(w, http.StatusBadRequest, "moderator_pubkey is required")
		return
	}

	cert, err := s.moderationEngine.DelegateModerator(r.Context(), req.Action, req.ModeratorPubKey, req.Alias, req.DurationSec)
	if err != nil {
		if errors.Is(err, moderation.ErrUntrustedModerator) {
			writeJSONError(w, http.StatusForbidden, "node is not configured with root admin credentials")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, cert)
}

// handleApplyKeyRotation verifies and records a root admin or release key rotation.
func (s *Server) handleApplyKeyRotation(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "key rotation submission is disabled in headless mode")
		return
	}
	if s.moderationEngine == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "moderation engine not initialized")
		return
	}

	var evt protocol.KeyRotationEvent
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid key rotation event json")
		return
	}

	if err := s.moderationEngine.ApplyKeyRotation(r.Context(), &evt); err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("failed to apply key rotation: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "applied",
		"event_id": evt.EventID,
	})
}

// handleExportIdentity exports the local node private key inside an encrypted PBKDF2+AES-GCM bundle.
func (s *Server) handleExportIdentity(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "identity export is disabled in headless mode")
		return
	}
	if s.publisherKey == nil {
		writeJSONError(w, http.StatusInternalServerError, "node identity private key is unavailable")
		return
	}

	var req ExportIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Passphrase) < 8 {
		writeJSONError(w, http.StatusBadRequest, "passphrase must be at least 8 characters")
		return
	}

	bundle, err := crypto.ExportEncryptedKey(s.publisherKey, req.Passphrase)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to export encrypted key: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bundle)
}

// handleImportIdentity restores an identity key from an encrypted bundle, updating node_key.ed25519.
func (s *Server) handleImportIdentity(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Headless {
		writeJSONError(w, http.StatusForbidden, "identity import is disabled in headless mode")
		return
	}

	var req ImportIdentityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Passphrase == "" {
		writeJSONError(w, http.StatusBadRequest, "passphrase is required")
		return
	}

	privKey, err := crypto.ImportEncryptedKey(req.Bundle, req.Passphrase)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid passphrase or corrupted key bundle: %v", err))
		return
	}

	keyPath := filepath.Join(s.cfg.DataDir, "node_key.ed25519")
	bakPath := filepath.Join(s.cfg.DataDir, "node_key.ed25519.bak")

	// Backup current key if it exists
	if oldData, err := os.ReadFile(keyPath); err == nil {
		_ = os.WriteFile(bakPath, oldData, 0600)
	}

	// Atomically save imported private key
	if err := crypto.SavePrivateKey(keyPath, privKey); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to save imported key: %v", err))
		return
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "imported",
		"restart_required": true,
		"public_key":       crypto.PublicKeyToHex(pubKey),
	})
}

// handleHeadlessStatusPage renders an audio-grade dark status page for headless swarm relays.
func (s *Server) handleHeadlessStatusPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	peerID := "unknown"
	if s.p2pHost != nil {
		peerID = s.p2pHost.ID().String()
	}

	connectedPeers := 0
	if s.p2pHost != nil {
		connectedPeers = len(s.p2pHost.ConnectedPeers())
	} else if s.peerTracker != nil {
		for _, p := range s.peerTracker.Peers() {
			if p.Connected {
				connectedPeers++
			}
		}
	}

	totalTracks, _, _ := s.catalog.Stats(r.Context())

	// Dynamically resolve public WAN multiaddress for WAN peers
	hostHeader := r.Host
	hostname, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		hostname = hostHeader
	}
	if hostname == "" {
		hostname = "127.0.0.1"
	}

	var wanMultiaddr string
	if ip := net.ParseIP(hostname); ip != nil {
		if ip.To4() != nil {
			wanMultiaddr = fmt.Sprintf("/ip4/%s/tcp/443/wss/p2p/%s", hostname, peerID)
		} else {
			wanMultiaddr = fmt.Sprintf("/ip6/%s/tcp/443/wss/p2p/%s", hostname, peerID)
		}
	} else {
		wanMultiaddr = fmt.Sprintf("/dns4/%s/tcp/443/wss/p2p/%s", hostname, peerID)
	}

	gistJSON := fmt.Sprintf("[\n  \"%s\"\n]", wanMultiaddr)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>ResonaMesh &bull; Swarm Relay Node</title>
    <style>
        :root {
            --bg: #0b0d10;
            --surface: #14171d;
            --border: #232834;
            --text-main: #f0f3f6;
            --text-muted: #8b949e;
            --accent: #00ffcc;
            --accent-glow: rgba(0, 255, 204, 0.15);
        }
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            background-color: var(--bg);
            color: var(--text-main);
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 24px;
        }
        .container {
            max-width: 680px;
            width: 100%%;
            background: var(--surface);
            border: 1px solid var(--border);
            border-radius: 16px;
            padding: 36px;
            box-shadow: 0 20px 40px rgba(0,0,0,0.4);
        }
        .header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 24px;
            padding-bottom: 20px;
            border-bottom: 1px solid var(--border);
        }
        .brand {
            display: flex;
            align-items: center;
            gap: 12px;
        }
        .logo-icon {
            width: 36px;
            height: 36px;
            border-radius: 8px;
            background: linear-gradient(135deg, #00ffcc 0%%, #0077ff 100%%);
            display: flex;
            align-items: center;
            justify-content: center;
            font-weight: 700;
            color: #0b0d10;
            font-size: 20px;
        }
        .brand h1 {
            font-size: 22px;
            font-weight: 700;
            letter-spacing: -0.5px;
        }
        .badge {
            display: inline-flex;
            align-items: center;
            gap: 6px;
            padding: 6px 12px;
            border-radius: 20px;
            font-size: 12px;
            font-weight: 600;
            background: var(--accent-glow);
            color: var(--accent);
            border: 1px solid rgba(0, 255, 204, 0.3);
        }
        .badge-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%%;
            background-color: var(--accent);
            box-shadow: 0 0 8px var(--accent);
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%% { opacity: 1; transform: scale(1); }
            50%% { opacity: 0.4; transform: scale(0.85); }
            100%% { opacity: 1; transform: scale(1); }
        }
        .lead {
            color: var(--text-muted);
            font-size: 14px;
            line-height: 1.6;
            margin-bottom: 28px;
        }
        .stat-grid {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 16px;
            margin-bottom: 24px;
        }
        .stat-item {
            background: #0d0f14;
            border: 1px solid var(--border);
            border-radius: 10px;
            padding: 16px;
            text-align: center;
        }
        .stat-val {
            font-size: 24px;
            font-weight: 700;
            color: var(--text-main);
        }
        .stat-lbl {
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: var(--text-muted);
            margin-top: 4px;
        }
        .card {
            background: #0d0f14;
            border: 1px solid var(--border);
            border-radius: 10px;
            padding: 16px;
            margin-bottom: 20px;
            overflow: hidden;
        }
        .card-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 8px;
        }
        .label {
            font-size: 11px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            color: var(--text-muted);
            font-weight: 600;
        }
        .copy-btn {
            background: #1a2230;
            border: 1px solid #2d3748;
            color: var(--accent);
            padding: 5px 12px;
            border-radius: 6px;
            font-size: 11px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.2s ease;
        }
        .copy-btn:hover {
            background: #232d40;
            border-color: var(--accent);
            box-shadow: 0 0 8px rgba(0, 255, 204, 0.3);
        }
        .copy-btn:active {
            transform: scale(0.97);
        }
        .code-box {
            background: #050608;
            border: 1px solid #1c212b;
            padding: 10px 14px;
            border-radius: 6px;
            font-family: ui-monospace, SFMono-Regular, "JetBrains Mono", Menlo, monospace;
            font-size: 13px;
            color: var(--accent);
            word-break: break-all;
            overflow-wrap: anywhere;
            white-space: pre-wrap;
            box-sizing: border-box;
            max-width: 100%%;
        }
        .security-note {
            background: rgba(255, 170, 0, 0.08);
            border: 1px solid rgba(255, 170, 0, 0.25);
            border-radius: 8px;
            padding: 14px 16px;
            font-size: 12px;
            color: #ffd280;
            line-height: 1.5;
            margin-bottom: 24px;
        }
        .footer {
            display: flex;
            align-items: center;
            justify-content: space-between;
            font-size: 12px;
            color: var(--text-muted);
        }
        .footer a {
            color: var(--accent);
            text-decoration: none;
        }
        .footer a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <div class="brand">
                <div class="logo-icon">&#x223F;</div>
                <div>
                    <h1>ResonaMesh</h1>
                    <div style="font-size: 12px; color: var(--text-muted);">Decentralized Audio Swarm Relay</div>
                </div>
            </div>
            <div class="badge">
                <div class="badge-dot"></div>
                SWARM RELAY ONLINE
            </div>
        </div>

        <p class="lead">
            This node operates as an autonomous, zero-storage bootstrap beacon and packet routing relay for the 
            ResonaMesh decentralized peer-to-peer audio network.
        </p>

        <div class="stat-grid">
            <div class="stat-item">
                <div class="stat-val">%d</div>
                <div class="stat-lbl">Connected Peers</div>
            </div>
            <div class="stat-item">
                <div class="stat-val">%d</div>
                <div class="stat-lbl">Tracks Indexed</div>
            </div>
            <div class="stat-item">
                <div class="stat-val">%s</div>
                <div class="stat-lbl">Version</div>
            </div>
        </div>

        <div class="card">
            <div class="card-header">
                <div class="label">Public Swarm Multiaddress (WebSocket Secure)</div>
                <button class="copy-btn" id="btn-copy-addr" onclick="copyText('wan-addr', 'btn-copy-addr', 'Copied Address!')">Copy Multiaddr</button>
            </div>
            <div class="code-box" id="wan-addr">%s</div>
        </div>

        <div class="card">
            <div class="card-header">
                <div class="label">Bootstrap Seeds JSON (Ready for GitHub Gist)</div>
                <button class="copy-btn" id="btn-copy-json" onclick="copyText('gist-json', 'btn-copy-json', 'Copied JSON!')">Copy for Gist</button>
            </div>
            <pre class="code-box" id="gist-json" style="white-space: pre-wrap; word-break: break-all; overflow-wrap: anywhere; line-height: 1.5;">%s</pre>
        </div>

        <div class="card">
            <div class="label" style="margin-bottom: 8px;">Swarm Peer Identity (LibP2P)</div>
            <div class="code-box">%s</div>
        </div>

        <div class="security-note">
            <strong>Headless Relay Mode Active:</strong> Interactive web mutations (audio file uploads, track deletions, 
            and configuration updates) are disabled on this public endpoint to protect node integrity.
        </div>

        <div class="footer">
            <span>ResonaMesh Autonomous Swarm Protocol</span>
            <a href="/api/v1/node/status" target="_blank">View JSON Metrics &rarr;</a>
        </div>
    </div>

    <script>
        function copyText(elemId, btnId, feedbackText) {
            var el = document.getElementById(elemId);
            if (!el) return;
            var text = el.innerText;
            navigator.clipboard.writeText(text).then(function() {
                var btn = document.getElementById(btnId);
                if (!btn) return;
                var orig = btn.innerText;
                btn.innerText = feedbackText;
                btn.style.borderColor = '#00ffcc';
                setTimeout(function() {
                    btn.innerText = orig;
                    btn.style.borderColor = '';
                }, 2000);
            });
        }
    </script>
</body>
</html>`, connectedPeers, totalTracks, AppVersion, wanMultiaddr, gistJSON, peerID)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}



// handleUpdaterStatus returns current updater telemetry, available releases, and download progress.
func (s *Server) handleUpdaterStatus(w http.ResponseWriter, r *http.Request) {
	if s.updaterEngine == nil {
		writeJSON(w, http.StatusOK, updater.State{
			Status:         updater.StatusIdle,
			CurrentVersion: AppVersion,
		})
		return
	}
	writeJSON(w, http.StatusOK, s.updaterEngine.State())
}

// handleUpdaterCheck queries connected swarm peers for newer release manifests.
func (s *Server) handleUpdaterCheck(w http.ResponseWriter, r *http.Request) {
	if s.releaseHandler != nil {
		_, _ = s.releaseHandler.QueryAllPeers(r.Context())
	}
	if s.updaterEngine == nil {
		writeJSON(w, http.StatusOK, updater.State{
			Status:         updater.StatusIdle,
			CurrentVersion: AppVersion,
		})
		return
	}
	writeJSON(w, http.StatusOK, s.updaterEngine.State())
}

// handleUpdaterDownload triggers the download and verification of available release chunks.
func (s *Server) handleUpdaterDownload(w http.ResponseWriter, r *http.Request) {
	if s.updaterEngine == nil {
		writeJSONError(w, http.StatusBadRequest, "updater not configured")
		return
	}

	state := s.updaterEngine.State()
	if state.Status != updater.StatusAvailable && state.Status != updater.StatusError {
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("cannot download in status %s", state.Status))
		return
	}

	// Trigger download in background context so HTTP call returns immediately with accepted status
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := s.updaterEngine.DownloadRelease(ctx); err != nil {
			slog.Error("Background release download failed", "error", err)
		}
	}()

	writeJSON(w, http.StatusAccepted, s.updaterEngine.State())
}

// handleUpdaterApply executes the atomic binary replacement and signals restart requirement.
func (s *Server) handleUpdaterApply(w http.ResponseWriter, r *http.Request) {
	if s.updaterEngine == nil {
		writeJSONError(w, http.StatusBadRequest, "updater not configured")
		return
	}

	if err := s.updaterEngine.ApplyUpdate(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to apply update: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "applied",
		"restart_required": true,
		"message":          "Update applied successfully. Please restart ResonaMesh to run the new version.",
	})
}

// handleUpdaterRollback restores the previous executable backup.
func (s *Server) handleUpdaterRollback(w http.ResponseWriter, r *http.Request) {
	if s.updaterEngine == nil {
		writeJSONError(w, http.StatusBadRequest, "updater not configured")
		return
	}

	if err := s.updaterEngine.Rollback(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed to rollback: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":           "rolled_back",
		"restart_required": true,
		"message":          "Previous binary restored successfully.",
	})
}
