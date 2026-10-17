package player

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"resonamesh/internal/catalog"
	"resonamesh/internal/storage"
	"resonamesh/pkg/protocol"
)

var (
	ErrBlockNotAvailable = errors.New("player: requested audio chunk not available in local cache")
)

// BlockFetcher retrieves missing audio blocks from the P2P swarm.
type BlockFetcher interface {
	FetchBlock(ctx context.Context, contentID string, blockIndex int, blockHash string) ([]byte, error)
}

// Streamer serves content-addressed audio streams to browser media clients via HTTP Range requests.
type Streamer struct {
	catalog       *catalog.Catalog
	blockStore    *storage.BlockStore
	fetcher       BlockFetcher
	activeTracker *storage.ActiveTracker
	quotaManager  *storage.QuotaManager
	activeStreams int32
}

// NewStreamer constructs an HTTP Range audio streaming handler.
func NewStreamer(cat *catalog.Catalog, bs *storage.BlockStore) *Streamer {
	return &Streamer{
		catalog:    cat,
		blockStore: bs,
	}
}

// SetFetcher sets the optional P2P swarm fetcher for on-demand streaming retrieval.
func (s *Streamer) SetFetcher(f BlockFetcher) {
	s.fetcher = f
}

// SetActiveTracker binds an ActiveTracker to prevent in-flight streaming chunks from being evicted by LRU.
func (s *Streamer) SetActiveTracker(at *storage.ActiveTracker) {
	s.activeTracker = at
}

// SetQuotaManager binds the QuotaManager to refresh block access timestamps on playback.
func (s *Streamer) SetQuotaManager(qm *storage.QuotaManager) {
	s.quotaManager = qm
}

// IsStreaming reports whether one or more active audio playback sessions are currently in progress.
func (s *Streamer) IsStreaming() bool {
	return atomic.LoadInt32(&s.activeStreams) > 0
}

// StreamTrack streams partial audio byte ranges for a specific content ID.
func (s *Streamer) StreamTrack(w http.ResponseWriter, r *http.Request, contentID string) {
	atomic.AddInt32(&s.activeStreams, 1)
	defer atomic.AddInt32(&s.activeStreams, -1)

	ctx := r.Context()

	track, err := s.catalog.GetTrack(ctx, contentID)
	if err != nil {
		if errors.Is(err, catalog.ErrTrackNotFound) {
			http.Error(w, "Track not found", http.StatusNotFound)
			return
		}
		slog.Error("Streamer failed to get track", "content_id", contentID, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	blockHashes, err := s.catalog.GetTrackBlockHashes(ctx, contentID)
	if err != nil || len(blockHashes) == 0 {
		http.Error(w, "Track audio blocks not found", http.StatusNotFound)
		return
	}

	totalSize := track.TotalSizeBytes
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "audio/mpeg")

	rangeHeader := r.Header.Get("Range")

	// 1. Full file or HEAD request without Range
	if rangeHeader == "" {
		w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusOK)
		if err := s.streamByteRange(ctx, w, contentID, blockHashes, 0, totalSize-1); err != nil {
			slog.Debug("Audio stream terminated prematurely", "content_id", contentID, "error", err)
		}
		return
	}

	// 2. Parse and validate Range header (bytes=start-end)
	start, end, err := parseRangeHeader(rangeHeader, totalSize)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
		http.Error(w, "Requested range not satisfiable", http.StatusRequestedRangeNotSatisfiable)
		return
	}

	contentLength := end - start + 1
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, totalSize))
	w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusPartialContent)
		return
	}

	w.WriteHeader(http.StatusPartialContent)
	if err := s.streamByteRange(ctx, w, contentID, blockHashes, start, end); err != nil {
		slog.Debug("Audio stream range terminated prematurely", "content_id", contentID, "error", err)
	}
}

// streamByteRange reads the exact sequence of 256 KiB chunks and slices the requested range.
func (s *Streamer) streamByteRange(ctx context.Context, w http.ResponseWriter, contentID string, blockHashes []string, start, end int64) error {
	blockSize := int64(protocol.BlockSize)

	startBlock := int(start / blockSize)
	endBlock := int(end / blockSize)

	if endBlock >= len(blockHashes) {
		endBlock = len(blockHashes) - 1
	}

	for b := startBlock; b <= endBlock; b++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		hash := blockHashes[b]
		if s.activeTracker != nil {
			s.activeTracker.Retain(hash)
			defer s.activeTracker.Release(hash)
		}
		if s.quotaManager != nil {
			_ = s.quotaManager.TouchBlock(ctx, hash)
		}

		chunkData, err := s.blockStore.Get(hash)
		if err != nil {
			if s.fetcher != nil {
				chunkData, err = s.fetcher.FetchBlock(ctx, contentID, b, hash)
			}
			if err != nil {
				return fmt.Errorf("%w: chunk index %d (hash %s)", ErrBlockNotAvailable, b, hash)
			}
		}

		// Calculate slice offsets inside this chunk
		var chunkStart int64 = 0
		if b == startBlock {
			chunkStart = start % blockSize
		}

		var chunkEnd int64 = int64(len(chunkData))
		if b == endBlock {
			chunkEnd = (end % blockSize) + 1
			if chunkEnd > int64(len(chunkData)) {
				chunkEnd = int64(len(chunkData))
			}
		}

		if chunkStart > chunkEnd || chunkStart >= int64(len(chunkData)) {
			continue
		}

		slice := chunkData[chunkStart:chunkEnd]
		if _, err := w.Write(slice); err != nil {
			return err
		}

		// Flush buffer if supported by response writer
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}

	return nil
}

// parseRangeHeader parses RFC 7233 Range headers into absolute byte offsets.
func parseRangeHeader(header string, totalSize int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, errors.New("invalid range prefix")
	}

	spec := strings.TrimPrefix(header, "bytes=")
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range format")
	}

	var start, end int64
	var err error

	if parts[0] == "" {
		// Suffix range: bytes=-500 (last 500 bytes)
		suffixLen, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffixLen <= 0 {
			return 0, 0, errors.New("invalid suffix length")
		}
		if suffixLen > totalSize {
			suffixLen = totalSize
		}
		start = totalSize - suffixLen
		end = totalSize - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || start < 0 || start >= totalSize {
			return 0, 0, errors.New("start offset out of bounds")
		}

		if parts[1] == "" {
			// Open-ended range: bytes=500-
			end = totalSize - 1
		} else {
			// Explicit range: bytes=500-1000
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil || end < start {
				return 0, 0, errors.New("end offset invalid")
			}
			if end >= totalSize {
				end = totalSize - 1
			}
		}
	}

	return start, end, nil
}
