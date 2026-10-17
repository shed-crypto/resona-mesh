package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/protocol"
)

var (
	ErrTrackNotFound = errors.New("catalog: track not found")
)

// Track represents metadata and local availability for a catalog audio item.
type Track struct {
	ContentID       string  `json:"content_id"`
	Title           string  `json:"title"`
	Artist          string  `json:"artist"`
	Description     string  `json:"description"`
	DurationMs      int64   `json:"duration_ms"`
	BitrateKbps     int     `json:"bitrate_kbps"`
	TotalSizeBytes  int64   `json:"total_size_bytes"`
	BlockSize       int     `json:"block_size"`
	BlockCount      int     `json:"block_count"`
	PublisherKey    string  `json:"publisher_key"`
	PublishedAt     int64   `json:"published_at"`
	LicenseCode     string  `json:"license_code"`
	Signature       string  `json:"signature"`
	Pinned          bool    `json:"pinned"`
	CreatedAt       int64   `json:"created_at"`
	AvailabilityPct float64 `json:"availability_pct"` // 0.0 - 100.0%
	Blocked         bool    `json:"blocked"`
	BlockReason     string  `json:"block_reason,omitempty"`
}

// Query specifies filtering, searching, and pagination parameters.
type Query struct {
	Search         string
	Limit          int
	Offset         int
	PinnedOnly     bool
	IncludeBlocked bool
}

// Catalog handles track querying, FTS5 full-text search, and availability computation.
type Catalog struct {
	db         *storage.DB
	blockStore *storage.BlockStore
}

// New constructs a Catalog instance.
func New(db *storage.DB, bs *storage.BlockStore) *Catalog {
	return &Catalog{
		db:         db,
		blockStore: bs,
	}
}

// ListTracks returns a paginated list of tracks with local block availability percentages.
func (c *Catalog) ListTracks(ctx context.Context, q Query) ([]Track, int, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 20
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	search := strings.TrimSpace(q.Search)

	var countQuery string
	var listQuery string
	var args []interface{}

	if search != "" {
		// FTS5 Full-Text Search
		ftsPattern := sanitizeFTSQuery(search)

		countQuery = `
			SELECT COUNT(*) 
			FROM tracks t
			JOIN tracks_fts fts ON t.content_id = fts.content_id
			LEFT JOIN blocked_content bc ON t.content_id = bc.content_id
			WHERE tracks_fts MATCH ?`
		if !q.IncludeBlocked {
			countQuery += " AND bc.content_id IS NULL"
		}
		if q.PinnedOnly {
			countQuery += " AND t.pinned = 1"
		}

		listQuery = `
			SELECT 
				t.content_id, t.title, t.artist, t.description, t.duration_ms, 
				t.bitrate_kbps, t.total_size_bytes, t.block_size, t.block_count, 
				t.publisher_key, t.published_at, t.license_code, t.signature, 
				t.pinned, t.created_at,
				(SELECT COUNT(*) FROM blocks b WHERE b.content_id = t.content_id AND b.is_stored = 1) AS stored_blocks,
				CASE WHEN bc.content_id IS NOT NULL THEN 1 ELSE 0 END AS is_blocked,
				COALESCE(bc.reason_text, '') AS block_reason
			FROM tracks t
			JOIN tracks_fts fts ON t.content_id = fts.content_id
			LEFT JOIN blocked_content bc ON t.content_id = bc.content_id
			WHERE tracks_fts MATCH ?`
		if !q.IncludeBlocked {
			listQuery += " AND bc.content_id IS NULL"
		}
		if q.PinnedOnly {
			listQuery += " AND t.pinned = 1"
		}
		listQuery += " ORDER BY t.created_at DESC LIMIT ? OFFSET ?"

		args = []interface{}{ftsPattern}
	} else {
		// Standard Catalog List
		countQuery = `
			SELECT COUNT(*) 
			FROM tracks t
			LEFT JOIN blocked_content bc ON t.content_id = bc.content_id
			WHERE 1=1`
		if !q.IncludeBlocked {
			countQuery += " AND bc.content_id IS NULL"
		}
		if q.PinnedOnly {
			countQuery += " AND t.pinned = 1"
		}

		listQuery = `
			SELECT 
				t.content_id, t.title, t.artist, t.description, t.duration_ms, 
				t.bitrate_kbps, t.total_size_bytes, t.block_size, t.block_count, 
				t.publisher_key, t.published_at, t.license_code, t.signature, 
				t.pinned, t.created_at,
				(SELECT COUNT(*) FROM blocks b WHERE b.content_id = t.content_id AND b.is_stored = 1) AS stored_blocks,
				CASE WHEN bc.content_id IS NOT NULL THEN 1 ELSE 0 END AS is_blocked,
				COALESCE(bc.reason_text, '') AS block_reason
			FROM tracks t
			LEFT JOIN blocked_content bc ON t.content_id = bc.content_id
			WHERE 1=1`
		if !q.IncludeBlocked {
			listQuery += " AND bc.content_id IS NULL"
		}
		if q.PinnedOnly {
			listQuery += " AND t.pinned = 1"
		}
		listQuery += " ORDER BY t.created_at DESC LIMIT ? OFFSET ?"
	}

	// 1. Get total count
	var totalCount int
	var countRow *sql.Row
	if len(args) > 0 {
		countRow = c.db.Raw().QueryRowContext(ctx, countQuery, args[0])
	} else {
		countRow = c.db.Raw().QueryRowContext(ctx, countQuery)
	}

	if err := countRow.Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("catalog: failed to count tracks: %w", err)
	}

	if totalCount == 0 {
		return []Track{}, 0, nil
	}

	// 2. Fetch paginated records
	listArgs := append(args, q.Limit, q.Offset)
	rows, err := c.db.Raw().QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("catalog: failed to list tracks: %w", err)
	}
	defer rows.Close()

	var tracks []Track
	for rows.Next() {
		var t Track
		var pinnedInt, storedBlocks, isBlockedInt int
		err := rows.Scan(
			&t.ContentID, &t.Title, &t.Artist, &t.Description, &t.DurationMs,
			&t.BitrateKbps, &t.TotalSizeBytes, &t.BlockSize, &t.BlockCount,
			&t.PublisherKey, &t.PublishedAt, &t.LicenseCode, &t.Signature,
			&pinnedInt, &t.CreatedAt, &storedBlocks,
			&isBlockedInt, &t.BlockReason,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("catalog: failed to scan track: %w", err)
		}
		t.Pinned = (pinnedInt == 1)
		t.Blocked = (isBlockedInt == 1)
		if t.BlockCount > 0 {
			t.AvailabilityPct = float64(storedBlocks) / float64(t.BlockCount) * 100.0
		}
		tracks = append(tracks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("catalog: error iterating track rows: %w", err)
	}

	return tracks, totalCount, nil
}

// GetTrack retrieves full metadata and availability for a single track.
func (c *Catalog) GetTrack(ctx context.Context, contentID string) (*Track, error) {
	query := `
		SELECT 
			t.content_id, t.title, t.artist, t.description, t.duration_ms, 
			t.bitrate_kbps, t.total_size_bytes, t.block_size, t.block_count, 
			t.publisher_key, t.published_at, t.license_code, t.signature, 
			t.pinned, t.created_at,
			(SELECT COUNT(*) FROM blocks b WHERE b.content_id = t.content_id AND b.is_stored = 1) AS stored_blocks,
			CASE WHEN bc.content_id IS NOT NULL THEN 1 ELSE 0 END AS is_blocked,
			COALESCE(bc.reason_text, '') AS block_reason
		FROM tracks t
		LEFT JOIN blocked_content bc ON t.content_id = bc.content_id
		WHERE t.content_id = ?`

	var t Track
	var pinnedInt, storedBlocks, isBlockedInt int
	err := c.db.Raw().QueryRowContext(ctx, query, contentID).Scan(
		&t.ContentID, &t.Title, &t.Artist, &t.Description, &t.DurationMs,
		&t.BitrateKbps, &t.TotalSizeBytes, &t.BlockSize, &t.BlockCount,
		&t.PublisherKey, &t.PublishedAt, &t.LicenseCode, &t.Signature,
		&pinnedInt, &t.CreatedAt, &storedBlocks,
		&isBlockedInt, &t.BlockReason,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTrackNotFound
		}
		return nil, fmt.Errorf("catalog: failed to get track: %w", err)
	}

	t.Pinned = (pinnedInt == 1)
	t.Blocked = (isBlockedInt == 1)
	if t.BlockCount > 0 {
		t.AvailabilityPct = float64(storedBlocks) / float64(t.BlockCount) * 100.0
	}
	return &t, nil
}

// GetManifest reconstructs the signed Manifest including all ordered block hashes.
func (c *Catalog) GetManifest(ctx context.Context, contentID string) (*protocol.Manifest, error) {
	t, err := c.GetTrack(ctx, contentID)
	if err != nil {
		return nil, err
	}

	blockHashes, err := c.GetTrackBlockHashes(ctx, contentID)
	if err != nil {
		return nil, err
	}

	return &protocol.Manifest{
		SchemaVersion:   "1.0.0",
		ContentID:       t.ContentID,
		Title:           t.Title,
		Artist:          t.Artist,
		DurationMs:      t.DurationMs,
		BitrateKbps:     t.BitrateKbps,
		TotalSizeBytes:  t.TotalSizeBytes,
		BlockSize:       t.BlockSize,
		BlockHashes:     blockHashes,
		PublisherKeyHex: t.PublisherKey,
		PublishedAt:     t.PublishedAt,
		LicenseCode:     t.LicenseCode,
		Signature:       t.Signature,
	}, nil
}

// ListManifests returns up to 100 track manifests currently registered in the catalog, excluding blocked content.
func (c *Catalog) ListManifests(ctx context.Context) ([]*protocol.Manifest, error) {
	query := `
		SELECT content_id 
		FROM tracks 
		WHERE content_id NOT IN (SELECT content_id FROM blocked_content)
		ORDER BY created_at DESC 
		LIMIT 100`
	rows, err := c.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("catalog: failed to list track IDs: %w", err)
	}
	defer rows.Close()

	var manifests []*protocol.Manifest
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		m, err := c.GetManifest(ctx, id)
		if err != nil {
			continue
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// RegisterRemoteManifest inserts an authenticated swarm manifest with unseeded local blocks.
func (c *Catalog) RegisterRemoteManifest(ctx context.Context, m *protocol.Manifest) error {
	var isBlocked int
	_ = c.db.Raw().QueryRowContext(ctx, "SELECT 1 FROM blocked_content WHERE content_id = ?", m.ContentID).Scan(&isBlocked)
	if isBlocked == 1 {
		return fmt.Errorf("catalog: cannot register remote manifest; content is blocked by policy")
	}

	var exists int
	err := c.db.Raw().QueryRowContext(ctx, "SELECT 1 FROM tracks WHERE content_id = ?", m.ContentID).Scan(&exists)
	if err == nil {
		return nil // already known in local catalog
	}

	now := time.Now().Unix()
	tx, err := c.db.Raw().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("catalog: failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	insertTrackSQL := `
	INSERT INTO tracks (
		content_id, title, artist, description, duration_ms, bitrate_kbps,
		total_size_bytes, block_size, block_count, publisher_key, published_at,
		license_code, signature, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`

	_, err = tx.ExecContext(ctx, insertTrackSQL,
		m.ContentID,
		m.Title,
		m.Artist,
		"",
		m.DurationMs,
		m.BitrateKbps,
		m.TotalSizeBytes,
		m.BlockSize,
		len(m.BlockHashes),
		m.PublisherKeyHex,
		m.PublishedAt,
		m.LicenseCode,
		m.Signature,
		now,
	)
	if err != nil {
		return fmt.Errorf("catalog: failed to insert remote track: %w", err)
	}

	insertBlockSQL := `
	INSERT INTO blocks (
		content_id, block_index, block_hash, size_bytes, is_stored, last_accessed_at
	) VALUES (?, ?, ?, 0, 0, ?)`

	stmtBlock, err := tx.PrepareContext(ctx, insertBlockSQL)
	if err != nil {
		return fmt.Errorf("catalog: failed to prepare block statement: %w", err)
	}
	defer stmtBlock.Close()

	for idx, hash := range m.BlockHashes {
		if _, err := stmtBlock.ExecContext(ctx, m.ContentID, idx, hash, now); err != nil {
			return fmt.Errorf("catalog: failed to insert remote block: %w", err)
		}
	}

	return tx.Commit()
}

// GetTrackBlockHashes returns the ordered sequence of chunk hashes for a track.
func (c *Catalog) GetTrackBlockHashes(ctx context.Context, contentID string) ([]string, error) {
	query := "SELECT block_hash FROM blocks WHERE content_id = ? ORDER BY block_index ASC"
	rows, err := c.db.Raw().QueryContext(ctx, query, contentID)
	if err != nil {
		return nil, fmt.Errorf("catalog: failed to query track blocks: %w", err)
	}
	defer rows.Close()

	var hashes []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("catalog: failed to scan block hash: %w", err)
		}
		hashes = append(hashes, h)
	}

	if len(hashes) == 0 {
		return nil, ErrTrackNotFound
	}
	return hashes, nil
}

// SetPinned updates the local retention pin status for a track.
func (c *Catalog) SetPinned(ctx context.Context, contentID string, pinned bool) error {
	val := 0
	if pinned {
		val = 1
	}
	res, err := c.db.Raw().ExecContext(ctx, "UPDATE tracks SET pinned = ? WHERE content_id = ?", val, contentID)
	if err != nil {
		return fmt.Errorf("catalog: failed to update pin status: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return ErrTrackNotFound
	}
	return nil
}

// MarkBlockStored marks a block as physically present in local storage.
func (c *Catalog) MarkBlockStored(ctx context.Context, contentID string, blockIndex int, sizeBytes int) error {
	now := time.Now().Unix()
	_, err := c.db.Raw().ExecContext(ctx, `
		UPDATE blocks 
		SET is_stored = 1, size_bytes = ?, last_accessed_at = ? 
		WHERE content_id = ? AND block_index = ?`,
		sizeBytes, now, contentID, blockIndex,
	)
	if err != nil {
		return fmt.Errorf("catalog: failed to mark block as stored: %w", err)
	}
	return nil
}

// DeleteTrack removes a track and its block index records from the database.
func (c *Catalog) DeleteTrack(ctx context.Context, contentID string) error {
	res, err := c.db.Raw().ExecContext(ctx, "DELETE FROM tracks WHERE content_id = ?", contentID)
	if err != nil {
		return fmt.Errorf("catalog: failed to delete track: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil || rowsAffected == 0 {
		return ErrTrackNotFound
	}
	return nil
}

// Stats returns total number of tracks and sum of stored block bytes.
func (c *Catalog) Stats(ctx context.Context) (totalTracks int, storedBytes int64, err error) {
	row := c.db.Raw().QueryRowContext(ctx, `
		SELECT 
			(SELECT COUNT(*) FROM tracks),
			COALESCE((SELECT SUM(size_bytes) FROM blocks WHERE is_stored = 1), 0)`)
	err = row.Scan(&totalTracks, &storedBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("catalog: failed to query stats: %w", err)
	}
	return totalTracks, storedBytes, nil
}


// sanitizeFTSQuery prepares a user query for safe SQLite FTS5 MATCH execution with wildcard suffix.
func sanitizeFTSQuery(term string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '"' || r == '*' || r == ':' || r == '^' {
			return -1
		}
		return r
	}, term)

	words := strings.Fields(cleaned)
	if len(words) == 0 {
		return ""
	}

	var parts []string
	for _, w := range words {
		parts = append(parts, w+"*")
	}
	return strings.Join(parts, " ")
}
