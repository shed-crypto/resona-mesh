package catalog_test

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"resonamesh/internal/catalog"
	"resonamesh/internal/importer"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func setupTestCatalog(t *testing.T) (*catalog.Catalog, *importer.Importer, *storage.DB) {
	t.Helper()
	tmpDir := t.TempDir()

	db, err := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	return cat, imp, db
}

func TestCatalogListAndPagination(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()

	// Import 5 distinct tracks
	for i := 1; i <= 5; i++ {
		audioData := bytes.Repeat([]byte{byte(i)}, 100*1024)
		_, err := imp.Import(ctx, importer.ImportParams{
			Title:        fmt.Sprintf("Audio Track %d", i),
			Artist:       fmt.Sprintf("Sound Artist %d", i),
			Description:  fmt.Sprintf("Clean synthesis sample %d", i),
			DurationMs:   int64(100000 + i*1000),
			BitrateKbps:  192,
			LicenseCode:  protocol.LicenseCC0,
			PublisherKey: privKey,
			AudioData:    bytes.NewReader(audioData),
		})
		if err != nil {
			t.Fatalf("failed to import track %d: %v", i, err)
		}
	}

	// Page 1: Limit 2, Offset 0
	tracks, total, err := cat.ListTracks(ctx, catalog.Query{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("ListTracks page 1 failed: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected total count 5, got %d", total)
	}
	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks on page 1, got %d", len(tracks))
	}
	if tracks[0].AvailabilityPct != 100.0 {
		t.Fatalf("expected 100%% local availability for imported track, got %f", tracks[0].AvailabilityPct)
	}

	// Page 2: Limit 2, Offset 2
	page2, _, err := cat.ListTracks(ctx, catalog.Query{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("ListTracks page 2 failed: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("expected 2 tracks on page 2, got %d", len(page2))
	}

	// Verify page 1 and page 2 contain different tracks
	if page2[0].ContentID == tracks[0].ContentID || page2[0].ContentID == tracks[1].ContentID {
		t.Fatal("pagination offset returned duplicate track across pages")
	}

	// Page 3: Limit 2, Offset 4 -> 1 track
	page3, _, err := cat.ListTracks(ctx, catalog.Query{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("ListTracks page 3 failed: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("expected 1 track on page 3, got %d", len(page3))
	}
}

func TestCatalogFTS5MultilingualSearch(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()

	// Track A (English)
	_, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Cyberpunk City Nights",
		Artist:       "Neon Horizon",
		Description:  "Atmospheric modular synthesizer soundtrack",
		DurationMs:   240000,
		BitrateKbps:  320,
		LicenseCode:  protocol.LicenseCCBY40,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(bytes.Repeat([]byte{0xCC}, 150*1024)),
	})
	if err != nil {
		t.Fatalf("failed to import track A: %v", err)
	}

	// Track B (Ukrainian)
	_, err = imp.Import(ctx, importer.ImportParams{
		Title:        "Мелодія карпатської сопілки",
		Artist:       "Василь Гуцул",
		Description:  "Традиційний фольклорний інструментальний твір",
		DurationMs:   180000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCCBY40,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(bytes.Repeat([]byte{0xAA}, 150*1024)),
	})
	if err != nil {
		t.Fatalf("failed to import track B: %v", err)
	}

	// Track C (Ukrainian)
	_, err = imp.Import(ctx, importer.ImportParams{
		Title:        "Вечірній джаз у парку",
		Artist:       "Тарас Шевченко",
		Description:  "Саксофон та контрабас на заході сонця",
		DurationMs:   210000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(bytes.Repeat([]byte{0xBB}, 150*1024)),
	})
	if err != nil {
		t.Fatalf("failed to import track C: %v", err)
	}

	// 1. English title prefix search
	results, total, err := cat.ListTracks(ctx, catalog.Query{Search: "cyberpunk"})
	if err != nil {
		t.Fatalf("English search failed: %v", err)
	}
	if total != 1 || len(results) != 1 || results[0].Title != "Cyberpunk City Nights" {
		t.Fatalf("unexpected English search result: %v", results)
	}

	// 2. Ukrainian title prefix search
	results, total, err = cat.ListTracks(ctx, catalog.Query{Search: "сопілк"})
	if err != nil {
		t.Fatalf("Ukrainian prefix search failed: %v", err)
	}
	if total != 1 || len(results) != 1 || results[0].Title != "Мелодія карпатської сопілки" {
		t.Fatalf("unexpected Ukrainian search result: %v", results)
	}

	// 3. Ukrainian artist search
	results, total, err = cat.ListTracks(ctx, catalog.Query{Search: "Шевченко"})
	if err != nil {
		t.Fatalf("search for artist failed: %v", err)
	}
	if total != 1 || results[0].Artist != "Тарас Шевченко" {
		t.Fatalf("expected artist match for Шевченко, got: %v", results)
	}

	// 4. Ukrainian description keyword
	results, total, err = cat.ListTracks(ctx, catalog.Query{Search: "контрабас"})
	if err != nil {
		t.Fatalf("search for description failed: %v", err)
	}
	if total != 1 || results[0].Title != "Вечірній джаз у парку" {
		t.Fatalf("expected match for description keyword, got: %v", results)
	}

	// 5. Search with no matches
	results, total, err = cat.ListTracks(ctx, catalog.Query{Search: "nonExistentTerm"})
	if err != nil {
		t.Fatalf("empty search failed: %v", err)
	}
	if total != 0 || len(results) != 0 {
		t.Fatalf("expected 0 results for non-existent word, got %d", total)
	}
}

func TestCatalogGetTrackAndManifest(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()
	// 550 KiB audio -> 3 chunks (256, 256, 38 KiB)
	audioBytes := make([]byte, 550*1024)
	for i := range audioBytes {
		audioBytes[i] = byte((i * 17) % 256)
	}

	origManifest, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Ambient Dawn",
		Artist:       "Electronic Producer",
		Description:  "Ambient synthesizer recording",
		DurationMs:   150000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(audioBytes),
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// 1. Get single track
	track, err := cat.GetTrack(ctx, origManifest.ContentID)
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if track.Title != origManifest.Title || track.BlockCount != 3 {
		t.Fatalf("GetTrack metadata mismatch: %+v", track)
	}

	// 2. Reconstruct Manifest and verify block hashes order
	reconstructed, err := cat.GetManifest(ctx, origManifest.ContentID)
	if err != nil {
		t.Fatalf("GetManifest failed: %v", err)
	}

	if reconstructed.ContentID != origManifest.ContentID {
		t.Fatalf("content ID mismatch")
	}
	if len(reconstructed.BlockHashes) != len(origManifest.BlockHashes) {
		t.Fatalf("block hashes length mismatch: %d vs %d", len(reconstructed.BlockHashes), len(origManifest.BlockHashes))
	}
	for i := range reconstructed.BlockHashes {
		if reconstructed.BlockHashes[i] != origManifest.BlockHashes[i] {
			t.Fatalf("block hash at index %d mismatch: %s vs %s", i, reconstructed.BlockHashes[i], origManifest.BlockHashes[i])
		}
	}
}

func TestCatalogPinAndFilter(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()

	m, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Pinned Test Track",
		Artist:       "Artist",
		DurationMs:   100000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("audio")),
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Imported track is pinned by default
	track, _ := cat.GetTrack(ctx, m.ContentID)
	if !track.Pinned {
		t.Fatal("expected imported track to be pinned")
	}

	// Unpin track
	if err := cat.SetPinned(ctx, m.ContentID, false); err != nil {
		t.Fatalf("SetPinned false failed: %v", err)
	}

	track, _ = cat.GetTrack(ctx, m.ContentID)
	if track.Pinned {
		t.Fatal("expected track to be unpinned")
	}

	// Query with PinnedOnly = true should return 0 results
	pinnedList, total, err := cat.ListTracks(ctx, catalog.Query{PinnedOnly: true})
	if err != nil {
		t.Fatalf("ListTracks pinned failed: %v", err)
	}
	if total != 0 || len(pinnedList) != 0 {
		t.Fatalf("expected 0 pinned tracks, got total=%d", total)
	}
}

func TestCatalogDeleteTrack(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()

	m, err := imp.Import(ctx, importer.ImportParams{
		Title:        "To Delete",
		Artist:       "Artist",
		DurationMs:   100000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("delete-me")),
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if err := cat.DeleteTrack(ctx, m.ContentID); err != nil {
		t.Fatalf("DeleteTrack failed: %v", err)
	}

	_, err = cat.GetTrack(ctx, m.ContentID)
	if err != catalog.ErrTrackNotFound {
		t.Fatalf("expected ErrTrackNotFound after deletion, got: %v", err)
	}

	// FTS5 should return 0 results
	results, total, _ := cat.ListTracks(ctx, catalog.Query{Search: "Delete"})
	if total != 0 || len(results) != 0 {
		t.Fatalf("expected deleted track to be purged from FTS5 index, got total=%d", total)
	}
}
