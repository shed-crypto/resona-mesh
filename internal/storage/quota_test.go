package storage_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func setupTestStorage(t *testing.T, quotaBytes int64) (*storage.QuotaManager, *storage.BlockStore, *storage.DB, string) {
	t.Helper()
	tmpDir := t.TempDir()

	db, err := storage.Open(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	tracker := storage.NewActiveTracker()
	qm := storage.NewQuotaManager(quotaBytes, tmpDir, db, bs, tracker)

	return qm, bs, db, tmpDir
}

func insertMockTrackAndBlock(
	t *testing.T,
	db *storage.DB,
	bs *storage.BlockStore,
	contentID string,
	blockIndex int,
	pinned bool,
	lastAccessedAt int64,
) string {
	t.Helper()
	ctx := context.Background()

	// Insert track if not exists
	pinnedVal := 0
	if pinned {
		pinnedVal = 1
	}

	_, _ = db.Raw().ExecContext(ctx, `
		INSERT OR IGNORE INTO tracks(
			content_id, title, artist, duration_ms, bitrate_kbps,
			total_size_bytes, block_size, block_count, publisher_key,
			published_at, license_code, signature, pinned, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		contentID, "Test Title", "Test Artist", 60000, 192,
		protocol.BlockSize, protocol.BlockSize, 1, "test_pub_key",
		time.Now().Unix(), protocol.LicenseCC0, "sig", pinnedVal, time.Now().Unix(),
	)

	// Update pin status if track already existed
	_, _ = db.Raw().ExecContext(ctx, "UPDATE tracks SET pinned = ? WHERE content_id = ?", pinnedVal, contentID)

	// Create real block payload
	payload := make([]byte, protocol.BlockSize)
	for i := range payload {
		payload[i] = byte((i + blockIndex + int(lastAccessedAt)) % 256)
	}
	hash := crypto.HashChunkHex(payload)

	if err := bs.Put(hash, payload); err != nil {
		t.Fatalf("failed to store block payload: %v", err)
	}

	_, err := db.Raw().ExecContext(ctx, `
		INSERT OR REPLACE INTO blocks(
			content_id, block_index, block_hash, size_bytes, is_stored, last_accessed_at
		) VALUES (?, ?, ?, ?, 1, ?)`,
		contentID, blockIndex, hash, protocol.BlockSize, lastAccessedAt,
	)
	if err != nil {
		t.Fatalf("failed to insert block record: %v", err)
	}

	return hash
}

func TestQuotaManagerReserveAndRelease(t *testing.T) {
	ctx := context.Background()
	qm, _, _, _ := setupTestStorage(t, 10*1024*1024) // 10 MiB quota

	tok, err := qm.Reserve(ctx, 2*1024*1024)
	if err != nil {
		t.Fatalf("expected successful reservation, got: %v", err)
	}

	usage, err := qm.Usage(ctx)
	if err != nil {
		t.Fatalf("failed to query usage: %v", err)
	}

	if usage.ReservedBytes != 2*1024*1024 {
		t.Fatalf("expected 2 MiB reserved, got: %d", usage.ReservedBytes)
	}

	// Release reservation
	qm.Release(tok)

	usageAfter, _ := qm.Usage(ctx)
	if usageAfter.ReservedBytes != 0 {
		t.Fatalf("expected 0 reserved after release, got: %d", usageAfter.ReservedBytes)
	}
}

func TestQuotaManagerLRUEvictionUnpinned(t *testing.T) {
	ctx := context.Background()
	// Quota is 512 KiB (exactly 2 protocol blocks of 256 KiB)
	quota := int64(2 * protocol.BlockSize)
	qm, bs, db, _ := setupTestStorage(t, quota)

	now := time.Now().Unix()
	// Insert 2 blocks for an unpinned track
	hashOld := insertMockTrackAndBlock(t, db, bs, "track_old", 0, false, now-100)
	hashNew := insertMockTrackAndBlock(t, db, bs, "track_new", 0, false, now)

	if !bs.Has(hashOld) || !bs.Has(hashNew) {
		t.Fatal("expected both initial blocks to exist on disk")
	}

	// Stored bytes should equal quota exactly
	usage, err := qm.Usage(ctx)
	if err != nil || usage.StoredBytes != quota {
		t.Fatalf("expected %d stored bytes, got: %d (err: %v)", quota, usage.StoredBytes, err)
	}

	// Reserve space for 1 more block (256 KiB). Must trigger LRU eviction of hashOld!
	tok, err := qm.Reserve(ctx, protocol.BlockSize)
	if err != nil {
		t.Fatalf("expected reservation to succeed with LRU eviction, got: %v", err)
	}
	defer qm.Release(tok)

	// Old block must have been evicted from disk and marked unstored
	if bs.Has(hashOld) {
		t.Fatal("expected older unpinned block to be deleted from disk")
	}
	// Newer block must remain
	if !bs.Has(hashNew) {
		t.Fatal("expected newer block to be retained on disk")
	}

	var isStored int
	_ = db.Raw().QueryRowContext(ctx, "SELECT is_stored FROM blocks WHERE block_hash = ?", hashOld).Scan(&isStored)
	if isStored != 0 {
		t.Fatalf("expected evicted block is_stored = 0, got: %d", isStored)
	}
}

func TestQuotaManagerLRUProtectPinned(t *testing.T) {
	ctx := context.Background()
	quota := int64(2 * protocol.BlockSize)
	qm, bs, db, _ := setupTestStorage(t, quota)

	now := time.Now().Unix()
	// Both blocks are PINNED
	hash1 := insertMockTrackAndBlock(t, db, bs, "track_pinned_1", 0, true, now-100)
	hash2 := insertMockTrackAndBlock(t, db, bs, "track_pinned_2", 0, true, now)

	// Try to reserve 1 more block. Since both stored blocks are pinned, none can be evicted.
	_, err := qm.Reserve(ctx, protocol.BlockSize)
	if err != storage.ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded when all blocks are pinned, got: %v", err)
	}

	// Pinned blocks must stay intact
	if !bs.Has(hash1) || !bs.Has(hash2) {
		t.Fatal("pinned blocks must never be evicted")
	}
}

func TestQuotaManagerLRUProtectActiveReader(t *testing.T) {
	ctx := context.Background()
	quota := int64(2 * protocol.BlockSize)
	qm, bs, db, _ := setupTestStorage(t, quota)

	now := time.Now().Unix()
	// Track is unpinned, but hashOld is currently being streamed
	hashOld := insertMockTrackAndBlock(t, db, bs, "track_old", 0, false, now-100)
	hashNew := insertMockTrackAndBlock(t, db, bs, "track_new", 0, false, now)

	// Retain hashOld in ActiveTracker
	qm.ActiveTracker().Retain(hashOld)

	// Reserve space for 1 more block. hashOld cannot be evicted because of active reader!
	// Therefore, hashNew will be evicted instead.
	tok, err := qm.Reserve(ctx, protocol.BlockSize)
	if err != nil {
		t.Fatalf("expected reservation to succeed by evicting hashNew, got: %v", err)
	}
	defer qm.Release(tok)

	if !bs.Has(hashOld) {
		t.Fatal("active reader block must NOT be evicted")
	}
	if bs.Has(hashNew) {
		t.Fatal("inactive block should have been evicted")
	}

	// Release active reader
	qm.ActiveTracker().Release(hashOld)
}
