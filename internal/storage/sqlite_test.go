package storage_test

import (
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/storage"
)

func TestDatabaseMigrationAndTables(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	// Verify tables exist
	tables := []string{"tracks", "blocks", "tracks_fts"}
	for _, table := range tables {
		var name string
		err := db.Raw().QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s to exist after migration: %v", table, err)
		}
	}
}

func TestForeignKeyCascading(t *testing.T) {
	// Use temporary file database to verify pragmas and foreign keys
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite database: %v", err)
	}
	defer db.Close()

	// Insert parent track
	now := time.Now().Unix()
	_, err = db.Raw().Exec(`
		INSERT INTO tracks (content_id, title, artist, description, duration_ms, bitrate_kbps, total_size_bytes, block_size, block_count, publisher_key, published_at, license_code, signature, pinned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cid-parent", "Track Title", "Artist Name", "Description", 180000, 192, 1048576, 262144, 4, "pubkey123", now, "CC0", "sig123", 0, now,
	)
	if err != nil {
		t.Fatalf("failed to insert parent track: %v", err)
	}

	// Insert child block
	_, err = db.Raw().Exec(`
		INSERT INTO blocks (block_hash, content_id, block_index, size_bytes, is_stored, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		"hash-child-1", "cid-parent", 0, 262144, 1, now,
	)
	if err != nil {
		t.Fatalf("failed to insert child block: %v", err)
	}

	// Delete parent track
	_, err = db.Raw().Exec("DELETE FROM tracks WHERE content_id = ?", "cid-parent")
	if err != nil {
		t.Fatalf("failed to delete parent track: %v", err)
	}

	// Verify child block was cascade-deleted
	var count int
	err = db.Raw().QueryRow("SELECT COUNT(*) FROM blocks WHERE content_id = ?", "cid-parent").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query blocks count: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected child blocks to be cascade-deleted, got count %d", count)
	}
}

func TestFTS5AutomaticSynchronizationTriggers(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fts_test.db")

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()

	// 1. INSERT parent track and verify automatic FTS5 insertion
	_, err = db.Raw().Exec(`
		INSERT INTO tracks (content_id, title, artist, description, duration_ms, bitrate_kbps, total_size_bytes, block_size, block_count, publisher_key, published_at, license_code, signature, pinned, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"cid-fts-1", "Акустична гітара", "Микола Ткаченко", "Живий запис студійного виступу в Києві", 120000, 192, 524288, 262144, 2, "pubkey", now, "CC-BY-4.0", "sig", 0, now,
	)
	if err != nil {
		t.Fatalf("failed to insert track: %v", err)
	}

	var ftsCount int
	err = db.Raw().QueryRow("SELECT COUNT(*) FROM tracks_fts WHERE tracks_fts MATCH 'гітар*'").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS5 query failed: %v", err)
	}
	if ftsCount != 1 {
		t.Fatalf("expected FTS5 trigger to index new track on INSERT, got count %d", ftsCount)
	}

	// 2. UPDATE title and verify FTS5 reflects update
	_, err = db.Raw().Exec("UPDATE tracks SET title = ? WHERE content_id = ?", "Електрогітара соло", "cid-fts-1")
	if err != nil {
		t.Fatalf("failed to update track: %v", err)
	}

	err = db.Raw().QueryRow("SELECT COUNT(*) FROM tracks_fts WHERE tracks_fts MATCH 'електрогітар*'").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS5 query after update failed: %v", err)
	}
	if ftsCount != 1 {
		t.Fatalf("expected FTS5 to find updated title, got %d", ftsCount)
	}

	// Old title must no longer match
	err = db.Raw().QueryRow("SELECT COUNT(*) FROM tracks_fts WHERE tracks_fts MATCH 'акустична'").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS5 query for old title failed: %v", err)
	}
	if ftsCount != 0 {
		t.Fatalf("expected old title to be removed from FTS5, got %d", ftsCount)
	}

	// 3. DELETE track and verify FTS5 index is purged
	_, err = db.Raw().Exec("DELETE FROM tracks WHERE content_id = ?", "cid-fts-1")
	if err != nil {
		t.Fatalf("failed to delete track: %v", err)
	}

	err = db.Raw().QueryRow("SELECT COUNT(*) FROM tracks_fts WHERE content_id = ?", "cid-fts-1").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS5 query after delete failed: %v", err)
	}
	if ftsCount != 0 {
		t.Fatalf("expected FTS5 entry to be deleted after parent track deletion, got %d", ftsCount)
	}
}
