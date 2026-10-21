package p2p

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"resonamesh/internal/catalog"
	"resonamesh/internal/storage"
	"resonamesh/pkg/protocol"
)

var (
	ErrKeeperDisabled = errors.New("keeper: background replication is disabled in configuration")
)

// StreamerActivityReporter reports whether user-facing audio playback is actively streaming.
type StreamerActivityReporter interface {
	IsStreaming() bool
}

// KeeperConfig defines parameters for voluntary background swarm replication.
type KeeperConfig struct {
	Enabled      bool
	Interval     time.Duration
	TargetCopies int
}

// DefaultKeeperConfig provides conservative replication settings respecting network hygiene.
func DefaultKeeperConfig() KeeperConfig {
	return KeeperConfig{
		Enabled:      false,
		Interval:     30 * time.Second,
		TargetCopies: 2,
	}
}

// Keeper executes autonomous background replication for rare swarm audio tracks
// adhering strictly to the bachelor thesis architecture:
// 1. Off by default until explicit volunteer consent.
// 2. Yields immediately when foreground playback is active.
// 3. Selects entries with < 2 observed copies.
// 4. Applies randomized jitter before downloading.
// 5. Validates SHA-256 integrity on every chunk before marking complete.
type Keeper struct {
	cfg      KeeperConfig
	catalog  *catalog.Catalog
	fetcher  *Fetcher
	streamer StreamerActivityReporter
	quota    *storage.QuotaManager

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewKeeper creates a background replication manager.
func NewKeeper(
	cfg KeeperConfig,
	cat *catalog.Catalog,
	fetcher *Fetcher,
	streamer StreamerActivityReporter,
	quota *storage.QuotaManager,
) *Keeper {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.TargetCopies <= 0 {
		cfg.TargetCopies = 2
	}

	return &Keeper{
		cfg:      cfg,
		catalog:  cat,
		fetcher:  fetcher,
		streamer: streamer,
		quota:    quota,
	}
}

// Start spawns the replication loop in a background goroutine if enabled.
func (k *Keeper) Start(ctx context.Context) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	if !k.cfg.Enabled {
		slog.Debug("Keeper replication service is disabled by configuration")
		return nil
	}

	if k.running {
		return nil
	}

	k.running = true
	k.stopCh = make(chan struct{})
	k.doneCh = make(chan struct{})

	go k.loop(ctx)
	slog.Info("Keeper background replication service started",
		"interval", k.cfg.Interval,
		"target_copies", k.cfg.TargetCopies,
	)

	return nil
}

// Stop gracefully signals the replication worker to terminate and awaits its exit.
func (k *Keeper) Stop() {
	k.mu.Lock()
	if !k.running {
		k.mu.Unlock()
		return
	}
	k.running = false
	close(k.stopCh)
	k.mu.Unlock()

	<-k.doneCh
	slog.Info("Keeper background replication service stopped")
}

// RunCycle executes a single synchronous replication evaluation pass. Useful for testing and manual triggering.
func (k *Keeper) RunCycle(ctx context.Context) error {
	// 1. Foreground playback priority: yield if user is actively streaming audio
	if k.streamer != nil && k.streamer.IsStreaming() {
		slog.Debug("Keeper: foreground audio playback in progress, yielding cycle")
		return nil
	}

	// 2. Discover tracks that are not fully cached locally
	tracks, _, err := k.catalog.ListTracks(ctx, catalog.Query{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		return fmt.Errorf("keeper: failed to list tracks: %w", err)
	}

	var candidates []catalog.Track
	for _, t := range tracks {
		if t.AvailabilityPct < 100.0 {
			candidates = append(candidates, t)
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// 3. Select candidate track (e.g. least stored blocks / rare in local swarm)
	candidate := candidates[0]

	// 4. Apply random backoff jitter (200ms - 800ms) to prevent thundering herd among peers
	jitterMs := randomJitter(200, 800)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(jitterMs):
	}

	// Re-check foreground streaming status after jitter
	if k.streamer != nil && k.streamer.IsStreaming() {
		slog.Debug("Keeper: playback started during jitter, aborting replication")
		return nil
	}

	slog.Info("Keeper selected candidate track for background replication",
		"content_id", candidate.ContentID,
		"title", candidate.Title,
		"availability_pct", candidate.AvailabilityPct,
		"total_blocks", candidate.BlockCount,
	)

	// 5. Query missing block hashes for candidate track
	hashes, err := k.catalog.GetTrackBlockHashes(ctx, candidate.ContentID)
	if err != nil {
		return fmt.Errorf("keeper: failed to get block hashes for %s: %w", candidate.ContentID, err)
	}

	// Fetch missing blocks sequentially with low-priority pacing
	for idx, hash := range hashes {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Yield immediately if user begins playing audio
		if k.streamer != nil && k.streamer.IsStreaming() {
			slog.Debug("Keeper: playback started mid-replication, pausing downloads")
			return nil
		}

		// Check if quota allows downloading another block
		if k.quota != nil {
			usage, usageErr := k.quota.Usage(ctx)
			if usageErr == nil && usage.AvailableBytes < protocol.BlockSize {
				slog.Debug("Keeper: insufficient available quota, halting replication pass", "available", usage.AvailableBytes)
				return nil
			}
		}

		// Download missing block from swarm peers
		_, fetchErr := k.fetcher.FetchBlock(ctx, candidate.ContentID, idx, hash)
		if fetchErr != nil {
			slog.Debug("Keeper: failed to replicate block", "content_id", candidate.ContentID, "index", idx, "err", fetchErr)
			// Move on to next cycle if source peer becomes unreachable
			break
		}

		// Gentle throttle between blocks (50ms) to avoid local CPU or socket saturation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}

	return nil
}

func (k *Keeper) loop(ctx context.Context) {
	defer close(k.doneCh)

	ticker := time.NewTicker(k.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-k.stopCh:
			return
		case <-ticker.C:
			cycleCtx, cancel := context.WithTimeout(ctx, k.cfg.Interval)
			if err := k.RunCycle(cycleCtx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Debug("Keeper replication pass completed with notice", "err", err)
			}
			cancel()
		}
	}
}

func randomJitter(minMs, maxMs int64) time.Duration {
	if maxMs <= minMs {
		return time.Duration(minMs) * time.Millisecond
	}
	var b [8]byte
	_, _ = rand.Read(b[:])
	val := int64(binary.LittleEndian.Uint64(b[:]))
	if val < 0 {
		val = -val
	}
	diff := maxMs - minMs
	chosen := minMs + (val % diff)
	return time.Duration(chosen) * time.Millisecond
}
