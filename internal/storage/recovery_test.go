package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestRecoverStaleTempFiles(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()

	bs, _ := storage.NewBlockStore(tmpDir)

	// Create a stale temp file in blocks/ab/cd/
	shardDir := filepath.Join(tmpDir, "blocks", "ab", "cd")
	_ = os.MkdirAll(shardDir, 0700)
	tempPath := filepath.Join(shardDir, "chunk-write-old-temp")
	_ = os.WriteFile(tempPath, []byte("interrupted-data"), 0600)

	// Set ModTime to 10 minutes ago
	pastTime := time.Now().Add(-10 * time.Minute)
	_ = os.Chtimes(tempPath, pastTime, pastTime)

	report, err := storage.Recover(ctx, tmpDir, db, bs)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if report.StaleTempFilesCleaned != 1 {
		t.Fatalf("expected 1 stale temp file cleaned, got: %d", report.StaleTempFilesCleaned)
	}

	if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
		t.Fatal("expected stale temp file to be removed from filesystem")
	}
}

func TestRecoverMissingBlocks(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()

	bs, _ := storage.NewBlockStore(tmpDir)

	// Insert track and record block as is_stored = 1 in SQLite, but do NOT write file to disk
	dummyHash := strings.Repeat("f", 64)
	_, _ = db.Raw().ExecContext(ctx, `
		INSERT INTO tracks(
			content_id, title, artist, duration_ms, bitrate_kbps,
			total_size_bytes, block_size, block_count, publisher_key,
			published_at, license_code, signature, pinned, created_at
		) VALUES ('track_missing', 'Title', 'Artist', 1000, 192, 100, 100, 1, 'key', 1, 'CC0', 'sig', 0, 1)`)

	_, _ = db.Raw().ExecContext(ctx, `
		INSERT INTO blocks(content_id, block_index, block_hash, size_bytes, is_stored, last_accessed_at)
		VALUES ('track_missing', 0, ?, 100, 1, 1)`, dummyHash)

	report, err := storage.Recover(ctx, tmpDir, db, bs)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if report.MissingBlocksReset != 1 {
		t.Fatalf("expected 1 missing block reset, got: %d", report.MissingBlocksReset)
	}

	var isStored int
	_ = db.Raw().QueryRowContext(ctx, "SELECT is_stored FROM blocks WHERE block_hash = ?", dummyHash).Scan(&isStored)
	if isStored != 0 {
		t.Fatalf("expected is_stored to be reset to 0, got: %d", isStored)
	}
}

func TestRecoverOrphanBlocks(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()

	bs, _ := storage.NewBlockStore(tmpDir)

	// Write valid chunk to disk using BlockStore, but leave database with NO track or block reference
	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte(i % 256)
	}
	hash := crypto.HashChunkHex(chunk)
	if err := bs.Put(hash, chunk); err != nil {
		t.Fatalf("failed to put chunk: %v", err)
	}

	if !bs.Has(hash) {
		t.Fatal("expected chunk to be stored before recovery")
	}

	report, err := storage.Recover(ctx, tmpDir, db, bs)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if report.OrphanFilesRemoved != 1 {
		t.Fatalf("expected 1 orphan file removed, got: %d", report.OrphanFilesRemoved)
	}

	if bs.Has(hash) {
		t.Fatal("expected orphan chunk to be removed from disk")
	}
}

func TestRecoverCorruptedBlocks(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()

	bs, _ := storage.NewBlockStore(tmpDir)

	validChunk := []byte("valid-chunk-payload")
	hash := crypto.HashChunkHex(validChunk)

	// Write valid chunk first
	_ = bs.Put(hash, validChunk)

	// Corrupt file contents on disk
	shardDir := filepath.Join(tmpDir, "blocks", hash[0:2], hash[2:4])
	filePath := filepath.Join(shardDir, hash)
	_ = os.WriteFile(filePath, []byte("corrupted-tampered-data"), 0600)

	// Index in DB as is_stored = 1
	_, _ = db.Raw().ExecContext(ctx, `
		INSERT INTO tracks(
			content_id, title, artist, duration_ms, bitrate_kbps,
			total_size_bytes, block_size, block_count, publisher_key,
			published_at, license_code, signature, pinned, created_at
		) VALUES ('track_corrupt', 'Title', 'Artist', 1000, 192, 100, 100, 1, 'key', 1, 'CC0', 'sig', 0, 1)`)

	_, _ = db.Raw().ExecContext(ctx, `
		INSERT INTO blocks(content_id, block_index, block_hash, size_bytes, is_stored, last_accessed_at)
		VALUES ('track_corrupt', 0, ?, 100, 1, 1)`, hash)

	report, err := storage.Recover(ctx, tmpDir, db, bs)
	if err != nil {
		t.Fatalf("recovery failed: %v", err)
	}

	if report.CorruptedBlocksPurged != 1 {
		t.Fatalf("expected 1 corrupted block purged, got: %d", report.CorruptedBlocksPurged)
	}

	var isStored int
	_ = db.Raw().QueryRowContext(ctx, "SELECT is_stored FROM blocks WHERE block_hash = ?", hash).Scan(&isStored)
	if isStored != 0 {
		t.Fatalf("expected is_stored to be reset to 0 for corrupted block, got: %d", isStored)
	}
}
