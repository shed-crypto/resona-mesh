package importer

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"resonamesh/internal/audio"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

var (
	ErrMissingTitle     = errors.New("importer: track title cannot be empty")
	ErrMissingArtist    = errors.New("importer: track artist cannot be empty")
	ErrInvalidLicense   = errors.New("importer: unsupported license (allowed: CC0, CC-BY-4.0)")
	ErrMissingKey       = errors.New("importer: publisher private key is required")
	ErrNilAudioStream   = errors.New("importer: audio stream reader is nil")
	ErrTrackAlreadyExists = errors.New("importer: track with this content ID already exists")
)

type ImportParams struct {
	Title        string
	Artist       string
	Description  string
	Lyrics       string
	DurationMs   int64
	BitrateKbps  int
	LicenseCode  string
	PublisherKey ed25519.PrivateKey
	AudioData    io.Reader
	CoverData    []byte
	CoverMime    string
}

// BlockChecker evaluates whether audio content is blocked by moderation policy.
type BlockChecker interface {
	IsBlocked(ctx context.Context, contentID string, payloadHash string) (bool, string, error)
}

// Importer coordinates audio chunking, block storage, manifest signing, and catalog persistence.
type Importer struct {
	chunker      *audio.Chunker
	blockStore   *storage.BlockStore
	db           *storage.DB
	quota        *storage.QuotaManager
	blockChecker BlockChecker
}

// NewImporter constructs an audio importer instance.
func NewImporter(bs *storage.BlockStore, db *storage.DB) *Importer {
	return &Importer{
		chunker:    audio.NewChunker(),
		blockStore: bs,
		db:         db,
	}
}

// SetQuotaManager binds a storage quota manager to validate space reservations prior to block commits.
func (imp *Importer) SetQuotaManager(qm *storage.QuotaManager) {
	imp.quota = qm
}

// SetBlockChecker binds a moderation policy checker to guard against importing blocked content.
func (imp *Importer) SetBlockChecker(bc BlockChecker) {
	imp.blockChecker = bc
}

// Import processes a local audio stream into discrete signed blocks and commits it to the catalog.
func (imp *Importer) Import(ctx context.Context, params ImportParams) (*protocol.Manifest, error) {
	if strings.TrimSpace(params.Title) == "" {
		return nil, ErrMissingTitle
	}
	if strings.TrimSpace(params.Artist) == "" {
		return nil, ErrMissingArtist
	}
	if len(params.PublisherKey) != ed25519.PrivateKeySize {
		return nil, ErrMissingKey
	}
	if params.AudioData == nil {
		return nil, ErrNilAudioStream
	}

	normLicense := strings.ToUpper(strings.TrimSpace(params.LicenseCode))
	if normLicense != protocol.LicenseCC0 && normLicense != protocol.LicenseCCBY40 {
		return nil, ErrInvalidLicense
	}

	if params.BitrateKbps <= 0 {
		params.BitrateKbps = 192
	}

	// 1. Chunk audio stream into 256 KiB blocks
	chunks, contentID, totalBytes, err := imp.chunker.Split(params.AudioData)
	if err != nil {
		return nil, fmt.Errorf("importer: chunking failed: %w", err)
	}

	if imp.blockChecker != nil {
		blocked, reason, err := imp.blockChecker.IsBlocked(ctx, contentID, "")
		if err == nil && blocked {
			return nil, fmt.Errorf("importer: content is blocked by moderation policy: %s", reason)
		}
	}

	// Check if track already exists in database
	var existing string
	err = imp.db.Raw().QueryRowContext(ctx, "SELECT content_id FROM tracks WHERE content_id = ?", contentID).Scan(&existing)
	if err == nil {
		return nil, ErrTrackAlreadyExists
	}

	// 2. Reserve disk quota if QuotaManager is configured (accounting for temporary doubling)
	var resToken storage.ReservationToken
	if imp.quota != nil {
		tok, err := imp.quota.Reserve(ctx, totalBytes*2)
		if err != nil {
			return nil, fmt.Errorf("importer: storage reservation failed: %w", err)
		}
		resToken = tok
	}

	// 3. Persist blocks into BlockStore
	var storedHashes []string
	cleanupBlocks := func() {
		if imp.quota != nil && resToken.ID != "" {
			imp.quota.Release(resToken)
		}
		for _, h := range storedHashes {
			_ = imp.blockStore.Delete(h)
		}
	}

	for _, chunk := range chunks {
		if err := imp.blockStore.Put(chunk.Hash, chunk.Data); err != nil {
			cleanupBlocks()
			return nil, fmt.Errorf("importer: failed to store block %s: %w", chunk.Hash, err)
		}
		storedHashes = append(storedHashes, chunk.Hash)
	}

	// 3. Assemble and sign Manifest
	pubKey := params.PublisherKey.Public().(ed25519.PublicKey)
	pubKeyHex := crypto.PublicKeyToHex(pubKey)
	now := time.Now().Unix()

	manifest := &protocol.Manifest{
		SchemaVersion:   "1.0.0",
		ContentID:       contentID,
		Title:           params.Title,
		Artist:          params.Artist,
		DurationMs:      params.DurationMs,
		BitrateKbps:     params.BitrateKbps,
		TotalSizeBytes:  totalBytes,
		BlockSize:       protocol.BlockSize,
		BlockHashes:     storedHashes,
		PublisherKeyHex: pubKeyHex,
		PublishedAt:     now,
		LicenseCode:     normLicense,
	}

	canonicalPayload := CanonicalManifestPayload(manifest)
	sigBytes := crypto.SignWithDomain(params.PublisherKey, crypto.DomainManifest, []byte(canonicalPayload))
	manifest.Signature = hex.EncodeToString(sigBytes)

	// 4. Save metadata into SQLite in a single transaction
	tx, err := imp.db.Raw().BeginTx(ctx, nil)
	if err != nil {
		cleanupBlocks()
		return nil, fmt.Errorf("importer: failed to start database transaction: %w", err)
	}
	defer tx.Rollback()

	insertTrackSQL := `
	INSERT INTO tracks (
		content_id, title, artist, description, lyrics, duration_ms, bitrate_kbps,
		total_size_bytes, block_size, block_count, publisher_key, published_at,
		license_code, signature, pinned, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, insertTrackSQL,
		manifest.ContentID,
		manifest.Title,
		manifest.Artist,
		params.Description,
		params.Lyrics,
		manifest.DurationMs,
		manifest.BitrateKbps,
		manifest.TotalSizeBytes,
		manifest.BlockSize,
		len(manifest.BlockHashes),
		manifest.PublisherKeyHex,
		manifest.PublishedAt,
		manifest.LicenseCode,
		manifest.Signature,
		1, // pinned = 1 for locally imported audio
		now,
	)
	if err != nil {
		cleanupBlocks()
		return nil, fmt.Errorf("importer: failed to insert track metadata: %w", err)
	}

	insertBlockSQL := `
	INSERT INTO blocks (
		block_hash, content_id, block_index, size_bytes, is_stored, last_accessed_at
	) VALUES (?, ?, ?, ?, ?, ?)`

	stmtBlock, err := tx.PrepareContext(ctx, insertBlockSQL)
	if err != nil {
		cleanupBlocks()
		return nil, fmt.Errorf("importer: failed to prepare block insert statement: %w", err)
	}
	defer stmtBlock.Close()

	for idx, chunk := range chunks {
		_, err := stmtBlock.ExecContext(ctx,
			chunk.Hash,
			manifest.ContentID,
			idx,
			len(chunk.Data),
			1, // is_stored = 1
			now,
		)
		if err != nil {
			cleanupBlocks()
			return nil, fmt.Errorf("importer: failed to insert block record: %w", err)
		}
	}

	if len(params.CoverData) > 0 {
		mime := params.CoverMime
		if mime == "" {
			mime = "image/jpeg"
		}
		insertCoverSQL := `
		INSERT INTO track_covers (content_id, mime_type, data, created_at)
		VALUES (?, ?, ?, ?)`
		if _, err := tx.ExecContext(ctx, insertCoverSQL, manifest.ContentID, mime, params.CoverData, now); err != nil {
			cleanupBlocks()
			return nil, fmt.Errorf("importer: failed to insert cover art: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		cleanupBlocks()
		return nil, fmt.Errorf("importer: failed to commit import transaction: %w", err)
	}

	if imp.quota != nil && resToken.ID != "" {
		imp.quota.Commit(resToken, totalBytes)
	}

	return manifest, nil
}

// CanonicalManifestPayload produces a deterministic string representation for signing and verification.
func CanonicalManifestPayload(m *protocol.Manifest) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%s|%d|%s",
		m.SchemaVersion,
		m.ContentID,
		m.Title,
		m.Artist,
		m.DurationMs,
		m.BitrateKbps,
		m.TotalSizeBytes,
		m.PublisherKeyHex,
		m.PublishedAt,
		m.LicenseCode,
	)
}
