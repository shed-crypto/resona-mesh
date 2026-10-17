package importer_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"

	"resonamesh/internal/importer"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestImporterFullPipeline(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	dbPath := filepath.Join(tmpDir, "catalog.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	imp := importer.NewImporter(bs, db)

	// 600 KiB synthetic audio stream -> 3 chunks (256, 256, 88 KiB)
	const totalSize = 600 * 1024
	audioData := make([]byte, totalSize)
	for i := range audioData {
		audioData[i] = byte((i * 13) % 256)
	}

	pubKey, privKey, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	params := importer.ImportParams{
		Title:        "Echoes of Silence",
		Artist:       "Resona Ensemble",
		Description:  "Studio recording prepared for decentralized streaming verification",
		DurationMs:   195000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCCBY40,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(audioData),
	}

	manifest, err := imp.Import(ctx, params)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// 1. Verify Manifest Integrity
	if manifest.Title != params.Title || manifest.Artist != params.Artist {
		t.Fatalf("manifest metadata mismatch")
	}
	if len(manifest.BlockHashes) != 3 {
		t.Fatalf("expected 3 chunk hashes, got %d", len(manifest.BlockHashes))
	}
	if manifest.TotalSizeBytes != totalSize {
		t.Fatalf("expected total size %d, got %d", totalSize, manifest.TotalSizeBytes)
	}

	// 2. Verify Cryptographic Signature
	sigBytes, err := hex.DecodeString(manifest.Signature)
	if err != nil {
		t.Fatalf("failed to decode signature hex: %v", err)
	}
	canonicalPayload := importer.CanonicalManifestPayload(manifest)
	if !crypto.VerifyWithDomain(pubKey, crypto.DomainManifest, []byte(canonicalPayload), sigBytes) {
		t.Fatal("manifest cryptographic Ed25519 signature verification failed")
	}

	// 3. Verify Database Persistence (tracks & blocks)
	var title, license string
	var pinned, blockCount int
	err = db.Raw().QueryRow("SELECT title, license_code, pinned, block_count FROM tracks WHERE content_id = ?", manifest.ContentID).
		Scan(&title, &license, &pinned, &blockCount)
	if err != nil {
		t.Fatalf("failed to query track from db: %v", err)
	}
	if title != params.Title || license != protocol.LicenseCCBY40 || pinned != 1 || blockCount != 3 {
		t.Fatalf("db track record values mismatch")
	}

	var storedBlocksCount int
	err = db.Raw().QueryRow("SELECT COUNT(*) FROM blocks WHERE content_id = ? AND is_stored = 1", manifest.ContentID).Scan(&storedBlocksCount)
	if err != nil {
		t.Fatalf("failed to query blocks count: %v", err)
	}
	if storedBlocksCount != 3 {
		t.Fatalf("expected 3 stored blocks in db, got %d", storedBlocksCount)
	}

	// 4. Verify FTS5 Trigger Population
	var ftsCount int
	err = db.Raw().QueryRow("SELECT COUNT(*) FROM tracks_fts WHERE tracks_fts MATCH 'silence*'").Scan(&ftsCount)
	if err != nil {
		t.Fatalf("FTS5 query failed: %v", err)
	}
	if ftsCount != 1 {
		t.Fatalf("expected FTS5 trigger to index imported track, got %d", ftsCount)
	}

	// 5. Verify BlockStore Disk Storage
	for _, h := range manifest.BlockHashes {
		if !bs.Has(h) {
			t.Fatalf("expected block %s to exist in blockstore", h)
		}
		data, err := bs.Get(h)
		if err != nil {
			t.Fatalf("failed to read block %s from blockstore: %v", h, err)
		}
		if len(data) == 0 {
			t.Fatalf("block %s data is empty", h)
		}
	}
}

func TestImporterValidationErrors(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()
	bs, _ := storage.NewBlockStore(tmpDir)
	imp := importer.NewImporter(bs, db)

	_, privKey, _ := crypto.GenerateKeyPair()
	validData := []byte("audio-sample")

	t.Run("Missing title", func(t *testing.T) {
		_, err := imp.Import(ctx, importer.ImportParams{
			Title:        "",
			Artist:       "Artist",
			LicenseCode:  protocol.LicenseCC0,
			PublisherKey: privKey,
			AudioData:    bytes.NewReader(validData),
		})
		if err != importer.ErrMissingTitle {
			t.Fatalf("expected ErrMissingTitle, got: %v", err)
		}
	})

	t.Run("Missing artist", func(t *testing.T) {
		_, err := imp.Import(ctx, importer.ImportParams{
			Title:        "Title",
			Artist:       " ",
			LicenseCode:  protocol.LicenseCC0,
			PublisherKey: privKey,
			AudioData:    bytes.NewReader(validData),
		})
		if err != importer.ErrMissingArtist {
			t.Fatalf("expected ErrMissingArtist, got: %v", err)
		}
	})

	t.Run("Invalid license", func(t *testing.T) {
		_, err := imp.Import(ctx, importer.ImportParams{
			Title:        "Title",
			Artist:       "Artist",
			LicenseCode:  "ALL_RIGHTS_RESERVED",
			PublisherKey: privKey,
			AudioData:    bytes.NewReader(validData),
		})
		if err != importer.ErrInvalidLicense {
			t.Fatalf("expected ErrInvalidLicense, got: %v", err)
		}
	})

	t.Run("Nil audio stream", func(t *testing.T) {
		_, err := imp.Import(ctx, importer.ImportParams{
			Title:        "Title",
			Artist:       "Artist",
			LicenseCode:  protocol.LicenseCC0,
			PublisherKey: privKey,
			AudioData:    nil,
		})
		if err != importer.ErrNilAudioStream {
			t.Fatalf("expected ErrNilAudioStream, got: %v", err)
		}
	})
}

func TestImporterDuplicateRejection(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()
	bs, _ := storage.NewBlockStore(tmpDir)
	imp := importer.NewImporter(bs, db)

	_, privKey, _ := crypto.GenerateKeyPair()
	sampleAudio := []byte("sample-audio-data-for-duplicate-test")

	params := importer.ImportParams{
		Title:        "Duplicate Test",
		Artist:       "Artist",
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(sampleAudio),
	}

	// First import succeeds
	_, err := imp.Import(ctx, params)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	// Second import with identical audio must fail with ErrTrackAlreadyExists
	params.AudioData = bytes.NewReader(sampleAudio)
	_, err = imp.Import(ctx, params)
	if err != importer.ErrTrackAlreadyExists {
		t.Fatalf("expected ErrTrackAlreadyExists, got: %v", err)
	}
}

func TestImporterQuotaEnforcement(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "test.db"))
	defer db.Close()
	bs, _ := storage.NewBlockStore(tmpDir)

	// Set small quota of 100 KiB
	qm := storage.NewQuotaManager(100*1024, tmpDir, db, bs, nil)

	imp := importer.NewImporter(bs, db)
	imp.SetQuotaManager(qm)

	_, privKey, _ := crypto.GenerateKeyPair()
	// Audio size is 300 KiB -> exceeds 100 KiB quota
	sampleAudio := make([]byte, 300*1024)

	params := importer.ImportParams{
		Title:        "Oversized Track",
		Artist:       "Artist",
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(sampleAudio),
	}

	_, err := imp.Import(ctx, params)
	if err == nil {
		t.Fatal("expected import to fail due to quota limit, got nil error")
	}

	// Verify no tracks or blocks remained in storage
	var trackCount int
	_ = db.Raw().QueryRowContext(ctx, "SELECT COUNT(*) FROM tracks").Scan(&trackCount)
	if trackCount != 0 {
		t.Fatalf("expected 0 tracks in db after failed import, got: %d", trackCount)
	}
}
