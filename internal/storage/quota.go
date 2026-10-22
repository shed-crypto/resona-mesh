package storage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

var (
	ErrQuotaExceeded         = errors.New("storage: storage quota exceeded")
	ErrInsufficientDiskSpace = errors.New("storage: insufficient physical disk space on host filesystem")
	ErrInvalidReservation    = errors.New("storage: invalid or expired reservation token")
)

// ActiveTracker tracks block hashes currently in use by active audio streams to protect them from LRU eviction.
type ActiveTracker struct {
	mu     sync.RWMutex
	counts map[string]int
}

// NewActiveTracker creates a new thread-safe ActiveTracker instance.
func NewActiveTracker() *ActiveTracker {
	return &ActiveTracker{
		counts: make(map[string]int),
	}
}

// Retain increments the active reader count for a block hash.
func (at *ActiveTracker) Retain(hash string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	at.counts[hash]++
}

// Release decrements the active reader count for a block hash.
func (at *ActiveTracker) Release(hash string) {
	at.mu.Lock()
	defer at.mu.Unlock()
	cnt := at.counts[hash]
	if cnt <= 1 {
		delete(at.counts, hash)
	} else {
		at.counts[hash] = cnt - 1
	}
}

// IsActive reports whether a block is currently being read by an active stream.
func (at *ActiveTracker) IsActive(hash string) bool {
	at.mu.RLock()
	defer at.mu.RUnlock()
	return at.counts[hash] > 0
}

// ReservationToken represents a temporary disk space reservation for an in-flight write operation.
type ReservationToken struct {
	ID        string
	Bytes     int64
	CreatedAt time.Time
}

// UsageStats provides detailed metrics about current storage quota utilization.
type UsageStats struct {
	QuotaBytes     int64 `json:"quota_bytes"`
	StoredBytes    int64 `json:"stored_bytes"`
	ReservedBytes  int64 `json:"reserved_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	PinnedBytes    int64 `json:"pinned_bytes"`
	UnpinnedBytes  int64 `json:"unpinned_bytes"`
}

// QuotaManager coordinates storage quota enforcement, transactional space reservation, and LRU block eviction.
type QuotaManager struct {
	mu            sync.Mutex
	quotaBytes    int64
	dataDir       string
	db            *DB
	blockStore    *BlockStore
	activeTracker *ActiveTracker
	reservedBytes int64
	tokens        map[string]int64
}

// NewQuotaManager constructs a QuotaManager instance.
func NewQuotaManager(
	quotaBytes int64,
	dataDir string,
	db *DB,
	bs *BlockStore,
	tracker *ActiveTracker,
) *QuotaManager {
	if tracker == nil {
		tracker = NewActiveTracker()
	}
	return &QuotaManager{
		quotaBytes:    quotaBytes,
		dataDir:       dataDir,
		db:            db,
		blockStore:    bs,
		activeTracker: tracker,
		tokens:        make(map[string]int64),
	}
}

// ActiveTracker returns the underlying active reader tracker.
func (qm *QuotaManager) ActiveTracker() *ActiveTracker {
	return qm.activeTracker
}

// QuotaBytes returns the configured quota limit.
func (qm *QuotaManager) QuotaBytes() int64 {
	return qm.quotaBytes
}

// SetQuotaBytes updates the quota limit dynamically.
func (qm *QuotaManager) SetQuotaBytes(quota int64) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.quotaBytes = quota
}

// Reserve attempts to atomically reserve disk space for an incoming chunk or batch.
// If current utilization plus the reservation exceeds the quota, eligible unpinned blocks
// are evicted according to LRU policy. Returns ErrQuotaExceeded if space cannot be reclaimed.
func (qm *QuotaManager) Reserve(ctx context.Context, bytes int64) (ReservationToken, error) {
	if bytes <= 0 {
		return ReservationToken{}, nil
	}

	qm.mu.Lock()
	defer qm.mu.Unlock()

	// 1. Verify underlying physical OS disk space
	if qm.dataDir != "" {
		freeDisk, err := AvailableDiskSpace(qm.dataDir)
		if err == nil && freeDisk < uint64(bytes) {
			return ReservationToken{}, ErrInsufficientDiskSpace
		}
	}

	// 2. Query currently stored bytes in SQLite
	var storedBytes int64
	row := qm.db.Raw().QueryRowContext(ctx, "SELECT COALESCE(SUM(size_bytes), 0) FROM blocks WHERE is_stored = 1")
	if err := row.Scan(&storedBytes); err != nil {
		return ReservationToken{}, fmt.Errorf("quota: failed to query stored bytes: %w", err)
	}

	totalNeeded := storedBytes + qm.reservedBytes + bytes
	if totalNeeded > qm.quotaBytes {
		bytesToReclaim := totalNeeded - qm.quotaBytes
		reclaimed, err := qm.evictLRU(ctx, bytesToReclaim)
		if err != nil {
			return ReservationToken{}, fmt.Errorf("quota: lru eviction failed: %w", err)
		}
		storedBytes -= reclaimed
		totalNeeded = storedBytes + qm.reservedBytes + bytes
		if totalNeeded > qm.quotaBytes {
			return ReservationToken{}, ErrQuotaExceeded
		}
	}

	// 3. Issue reservation token
	tokenID := generateReservationID()
	qm.reservedBytes += bytes
	qm.tokens[tokenID] = bytes

	return ReservationToken{
		ID:        tokenID,
		Bytes:     bytes,
		CreatedAt: time.Now(),
	}, nil
}

// Release cancels an active reservation without committing bytes to permanent storage.
func (qm *QuotaManager) Release(token ReservationToken) {
	if token.ID == "" {
		return
	}
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if bytes, exists := qm.tokens[token.ID]; exists {
		qm.reservedBytes -= bytes
		if qm.reservedBytes < 0 {
			qm.reservedBytes = 0
		}
		delete(qm.tokens, token.ID)
	}
}

// Commit concludes an active reservation when blocks have been successfully written to disk.
func (qm *QuotaManager) Commit(token ReservationToken, actualBytes int64) {
	if token.ID == "" {
		return
	}
	qm.mu.Lock()
	defer qm.mu.Unlock()

	if bytes, exists := qm.tokens[token.ID]; exists {
		qm.reservedBytes -= bytes
		if qm.reservedBytes < 0 {
			qm.reservedBytes = 0
		}
		delete(qm.tokens, token.ID)
	}
}

// Usage computes the current storage quota distribution.
func (qm *QuotaManager) Usage(ctx context.Context) (UsageStats, error) {
	qm.mu.Lock()
	reserved := qm.reservedBytes
	quota := qm.quotaBytes
	qm.mu.Unlock()

	var storedBytes int64
	var pinnedBytes int64

	row := qm.db.Raw().QueryRowContext(ctx, `
		SELECT 
			COALESCE(SUM(b.size_bytes), 0),
			COALESCE(SUM(CASE WHEN t.pinned = 1 THEN b.size_bytes ELSE 0 END), 0)
		FROM blocks b
		JOIN tracks t ON b.content_id = t.content_id
		WHERE b.is_stored = 1
	`)
	if err := row.Scan(&storedBytes, &pinnedBytes); err != nil {
		return UsageStats{}, fmt.Errorf("quota: failed to query usage: %w", err)
	}

	available := quota - storedBytes - reserved
	if available < 0 {
		available = 0
	}

	unpinned := storedBytes - pinnedBytes
	if unpinned < 0 {
		unpinned = 0
	}

	return UsageStats{
		QuotaBytes:     quota,
		StoredBytes:    storedBytes,
		ReservedBytes:  reserved,
		AvailableBytes: available,
		PinnedBytes:    pinnedBytes,
		UnpinnedBytes:  unpinned,
	}, nil
}

// TouchBlock updates the last accessed timestamp for a block hash to refresh its LRU position.
func (qm *QuotaManager) TouchBlock(ctx context.Context, hash string) error {
	now := time.Now().Unix()
	_, err := qm.db.Raw().ExecContext(ctx, "UPDATE blocks SET last_accessed_at = ? WHERE block_hash = ?", now, hash)
	if err != nil {
		return fmt.Errorf("quota: failed to touch block: %w", err)
	}
	return nil
}

// evictLRU iterates over unpinned blocks ordered by oldest last_accessed_at and removes them from disk and database.
// Must be called with qm.mu held.
func (qm *QuotaManager) evictLRU(ctx context.Context, requiredBytes int64) (int64, error) {
	rows, err := qm.db.Raw().QueryContext(ctx, `
		SELECT b.content_id, b.block_index, b.block_hash, b.size_bytes
		FROM blocks b
		JOIN tracks t ON b.content_id = t.content_id
		WHERE b.is_stored = 1 AND t.pinned = 0
		ORDER BY b.last_accessed_at ASC
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to query eviction candidates: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		contentID  string
		blockIndex int
		blockHash  string
		sizeBytes  int64
	}

	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.contentID, &c.blockIndex, &c.blockHash, &c.sizeBytes); err != nil {
			return 0, err
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	var reclaimed int64
	for _, c := range candidates {
		// Never evict blocks with active readers
		if qm.activeTracker.IsActive(c.blockHash) {
			continue
		}

		// Delete physical block payload from disk
		if err := qm.blockStore.Delete(c.blockHash); err != nil {
			slog.Warn("Failed to delete evicted block file", "hash", c.blockHash, "err", err)
		}

		// Update database flag
		_, err := qm.db.Raw().ExecContext(ctx, `
			UPDATE blocks 
			SET is_stored = 0 
			WHERE content_id = ? AND block_index = ?`,
			c.contentID, c.blockIndex,
		)
		if err != nil {
			slog.Warn("Failed to update evicted block record", "content_id", c.contentID, "index", c.blockIndex, "err", err)
			continue
		}

		reclaimed += c.sizeBytes
		if reclaimed >= requiredBytes {
			break
		}
	}

	slog.Info("Completed LRU eviction pass",
		"required_bytes", requiredBytes,
		"reclaimed_bytes", reclaimed,
	)

	return reclaimed, nil
}

func generateReservationID() string {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}
