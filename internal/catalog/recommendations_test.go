package catalog_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"resonamesh/internal/catalog"
	"resonamesh/internal/importer"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestCatalogRecommendations(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	bs, err := storage.NewBlockStore(filepath.Join(tmpDir, "blocks"))
	if err != nil {
		t.Fatalf("failed to init block store: %v", err)
	}

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	ctx := context.Background()

	_, privKey, _ := crypto.GenerateKeyPair()

	// Seed Track A: Ambient, 90 BPM, tags: ["nordic", "chill"], artist: "Fjord Harmonic"
	mA, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Fjord Dawn",
		Artist:       "Fjord Harmonic",
		DurationMs:   180000,
		BitrateKbps:  320,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("unique audio stream A 1234567890")),
		Genre:        "Ambient",
		Tags:         []string{"nordic", "chill"},
		BPM:          90,
		Mood:         "Calm",
	})
	if err != nil {
		t.Fatalf("failed to import track A: %v", err)
	}

	// Track B: Ambient, 92 BPM, tags: ["nordic", "relax"], artist: "Fjord Harmonic" (Very High Similarity)
	mB, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Glacier Stream",
		Artist:       "Fjord Harmonic",
		DurationMs:   200000,
		BitrateKbps:  320,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("unique audio stream B 1234567890")),
		Genre:        "Ambient",
		Tags:         []string{"nordic", "relax"},
		BPM:          92,
		Mood:         "Calm",
	})
	if err != nil {
		t.Fatalf("failed to import track B: %v", err)
	}

	// Track C: Synthwave, 180 BPM (double time of 90 BPM), tags: ["nordic", "cyber"], artist: "Neon Pulse" (Moderate Similarity)
	mC, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Midnight Aurora",
		Artist:       "Neon Pulse",
		DurationMs:   210000,
		BitrateKbps:  320,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("unique audio stream C 1234567890")),
		Genre:        "Synthwave",
		Tags:         []string{"nordic", "cyber"},
		BPM:          180,
		Mood:         "Energetic",
	})
	if err != nil {
		t.Fatalf("failed to import track C: %v", err)
	}

	// Track D: Heavy Metal, 140 BPM, tags: ["distortion", "screams"], artist: "Thunder Skull" (Zero/Minimal Similarity)
	_, err = imp.Import(ctx, importer.ImportParams{
		Title:        "Iron Wrath",
		Artist:       "Thunder Skull",
		DurationMs:   240000,
		BitrateKbps:  320,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("unique audio stream D 1234567890")),
		Genre:        "Metal",
		Tags:         []string{"distortion", "screams"},
		BPM:          140,
		Mood:         "Aggressive",
	})
	if err != nil {
		t.Fatalf("failed to import track D: %v", err)
	}

	// Request recommendations for Track A
	recs, err := cat.GetRecommendations(ctx, mA.ContentID, 10)
	if err != nil {
		t.Fatalf("GetRecommendations failed: %v", err)
	}

	if len(recs) != 3 {
		t.Fatalf("expected 3 candidate recommendations, got %d", len(recs))
	}

	// 1. Verify seed track itself is excluded
	for _, r := range recs {
		if r.Track.ContentID == mA.ContentID {
			t.Fatalf("seed track %s must not be in its own recommendations", mA.ContentID)
		}
	}

	// 2. Track B must be rank #1 (highest similarity score)
	if recs[0].Track.ContentID != mB.ContentID {
		t.Errorf("expected rank 1 to be track B (%s), got %s", mB.ContentID, recs[0].Track.ContentID)
	}

	// 3. Track C must be rank #2 (tempo harmonic overlap + tag overlap)
	if recs[1].Track.ContentID != mC.ContentID {
		t.Errorf("expected rank 2 to be track C (%s), got %s", mC.ContentID, recs[1].Track.ContentID)
	}

	// 4. Verify score monotonicity
	if recs[0].Score <= recs[1].Score || recs[1].Score <= recs[2].Score {
		t.Errorf("expected strictly descending scores: %f > %f > %f",
			recs[0].Score, recs[1].Score, recs[2].Score)
	}
}
