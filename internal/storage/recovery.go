package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

// RecoveryReport summarizes the outcome of a storage reconciliation pass.
type RecoveryReport struct {
	StaleTempFilesCleaned int
	OrphanFilesRemoved    int
	MissingBlocksReset    int
	CorruptedBlocksPurged int
}

// Recover runs a startup reconciliation pass between the physical block store and SQLite.
// It removes leftover temp files from interrupted writes, verifies that stored database blocks
// actually exist on disk, and purges unindexed orphan chunk files from filesystem shards.
func Recover(ctx context.Context, dataDir string, db *DB, bs *BlockStore) (*RecoveryReport, error) {
	report := &RecoveryReport{}

	blocksDir := filepath.Join(dataDir, "blocks")
	if _, err := os.Stat(blocksDir); os.IsNotExist(err) {
		return report, nil
	}

	// 1. Clean up stale chunk-write-* temporary files
	err := filepath.WalkDir(blocksDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if strings.HasPrefix(name, "chunk-write-") {
			info, statErr := d.Info()
			// Remove temp file if created more than 1 minute ago (or on node boot)
			if statErr == nil && time.Since(info.ModTime()) > time.Minute {
				if removeErr := os.Remove(path); removeErr == nil {
					report.StaleTempFilesCleaned++
				}
			}
		}
		return nil
	})
	if err != nil {
		slog.Warn("Recovery: error scanning for temp files", "err", err)
	}

	// 2. Reconcile DB -> Disk: ensure blocks marked is_stored = 1 exist physically
	rows, err := db.Raw().QueryContext(ctx, "SELECT content_id, block_index, block_hash FROM blocks WHERE is_stored = 1")
	if err != nil {
		return report, fmt.Errorf("recovery: failed to query stored blocks: %w", err)
	}

	type blockEntry struct {
		contentID  string
		blockIndex int
		blockHash  string
	}

	var storedBlocks []blockEntry
	for rows.Next() {
		var b blockEntry
		if scanErr := rows.Scan(&b.contentID, &b.blockIndex, &b.blockHash); scanErr == nil {
			storedBlocks = append(storedBlocks, b)
		}
	}
	_ = rows.Close()

	for _, b := range storedBlocks {
		if !bs.Has(b.blockHash) {
			_, resetErr := db.Raw().ExecContext(ctx,
				"UPDATE blocks SET is_stored = 0 WHERE content_id = ? AND block_index = ?",
				b.contentID, b.blockIndex,
			)
			if resetErr == nil {
				report.MissingBlocksReset++
			}
		}
	}

	// 3. Collect release manifest blocks so updater blocks are never treated as orphans
	protectedReleaseBlocks := make(map[string]bool)
	updatesDir := filepath.Join(dataDir, "updates")
	if entries, readDirErr := os.ReadDir(updatesDir); readDirErr == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
				manifestPath := filepath.Join(updatesDir, e.Name())
				if data, readFileErr := os.ReadFile(manifestPath); readFileErr == nil {
					var rel protocol.ReleaseManifest
					if unmarshalErr := json.Unmarshal(data, &rel); unmarshalErr == nil {
						for _, h := range rel.BlockHashes {
							protectedReleaseBlocks[strings.ToLower(strings.TrimSpace(h))] = true
						}
					}
				}
			}
		}
	}

	// 4. Reconcile Disk -> DB: purge orphan or corrupted chunk files
	err = filepath.WalkDir(blocksDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		hash := d.Name()
		if strings.HasPrefix(hash, "chunk-write-") {
			return nil
		}

		// Valid chunk filenames are exactly 64 hex characters
		if len(hash) != 64 {
			return nil
		}

		normalizedHash := strings.ToLower(hash)
		if protectedReleaseBlocks[normalizedHash] {
			// Chunk belongs to an authentic application release manifest; verify cryptographic integrity
			data, readErr := os.ReadFile(path)
			if readErr != nil || !crypto.VerifyChunk(data, normalizedHash) {
				_ = os.Remove(path)
				report.CorruptedBlocksPurged++
			}
			return nil
		}

		var count int
		countErr := db.Raw().QueryRowContext(ctx, "SELECT COUNT(*) FROM blocks WHERE block_hash = ?", hash).Scan(&count)
		if countErr != nil || count == 0 {
			// Orphan block not indexed in database catalog
			if removeErr := os.Remove(path); removeErr == nil {
				report.OrphanFilesRemoved++
			}
			return nil
		}

		// Verify cryptographic integrity
		data, readErr := os.ReadFile(path)
		if readErr != nil || !crypto.VerifyChunk(data, hash) {
			_ = os.Remove(path)
			_, _ = db.Raw().ExecContext(ctx, "UPDATE blocks SET is_stored = 0 WHERE block_hash = ?", hash)
			report.CorruptedBlocksPurged++
		}

		return nil
	})
	if err != nil {
		slog.Warn("Recovery: error scanning blocks directory for orphans", "err", err)
	}

	slog.Info("Storage recovery completed successfully",
		"stale_temp_cleaned", report.StaleTempFilesCleaned,
		"orphans_removed", report.OrphanFilesRemoved,
		"missing_reset", report.MissingBlocksReset,
		"corrupted_purged", report.CorruptedBlocksPurged,
	)

	return report, nil
}
