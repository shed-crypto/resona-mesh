package catalog_test

import (
	"bytes"
	"context"
	"errors"
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

func TestCatalogModerationFiltering(t *testing.T) {
	ctx := context.Background()
	cat, imp, db := setupTestCatalog(t)

	_, privKey, _ := crypto.GenerateKeyPair()

	m, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Flagged Track",
		Artist:       "Copyright Infringer",
		DurationMs:   120000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader([]byte("sample-data-to-block")),
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// 1. Before blocking: track appears in normal ListTracks
	tracks, total, err := cat.ListTracks(ctx, catalog.Query{})
	if err != nil || total != 1 || len(tracks) != 1 {
		t.Fatalf("expected 1 unblocked track, got total=%d, err=%v", total, err)
	}
	if tracks[0].Blocked {
		t.Fatal("expected track to not be blocked initially")
	}

	// 2. Mark track as blocked in blocked_content
	insertBlockSQL := `
	INSERT INTO blocked_content (
		content_id, payload_hash, action, decision_event_id, reason_code, reason_text, blocked_at
	) VALUES (?, ?, 'BLOCK', 'dec-1', 'COPYRIGHT_CLAIM', 'DMCA violation', 1700000000)`
	_, err = db.Raw().ExecContext(ctx, insertBlockSQL, m.ContentID, "dummy-payload-hash")
	if err != nil {
		t.Fatalf("failed to insert block record: %v", err)
	}

	// 3. Normal ListTracks should now hide the blocked track
	tracks, total, err = cat.ListTracks(ctx, catalog.Query{})
	if err != nil || total != 0 || len(tracks) != 0 {
		t.Fatalf("expected 0 tracks in normal catalog after blocking, got total=%d", total)
	}

	// 4. ListTracks with IncludeBlocked should return the track with Blocked = true and reason
	tracks, total, err = cat.ListTracks(ctx, catalog.Query{IncludeBlocked: true})
	if err != nil || total != 1 || len(tracks) != 1 {
		t.Fatalf("expected 1 track with IncludeBlocked, got total=%d", total)
	}
	if !tracks[0].Blocked || tracks[0].BlockReason != "DMCA violation" {
		t.Fatalf("expected track blocked=true with reason, got blocked=%v, reason=%s", tracks[0].Blocked, tracks[0].BlockReason)
	}

	// 5. GetTrack directly retrieves track with Blocked = true
	track, err := cat.GetTrack(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if !track.Blocked || track.BlockReason != "DMCA violation" {
		t.Fatalf("expected GetTrack to reflect blocked status, got blocked=%v, reason=%s", track.Blocked, track.BlockReason)
	}

	// 6. RegisterRemoteManifest should reject re-registering blocked content
	err = cat.RegisterRemoteManifest(ctx, m)
	if err == nil {
		t.Fatal("expected RegisterRemoteManifest to fail for blocked track, got nil")
	}
}

func TestCatalogCoverArt(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)
	_, privKey, _ := crypto.GenerateKeyPair()

	audioData := make([]byte, 50*1024)
	coverData := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // Fake PNG header

	m, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Cover Track",
		Artist:       "Cover Artist",
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(audioData),
		CoverData:    coverData,
		CoverMime:    "image/png",
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// 1. Verify Track has HasCover = true
	track, err := cat.GetTrack(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if !track.HasCover {
		t.Fatal("expected track.HasCover to be true")
	}

	// 2. Verify ListTracks also reports HasCover = true
	tracks, total, err := cat.ListTracks(ctx, catalog.Query{})
	if err != nil || total != 1 || len(tracks) != 1 {
		t.Fatalf("ListTracks failed: total=%d, err=%v", total, err)
	}
	if !tracks[0].HasCover {
		t.Fatal("expected ListTracks track.HasCover to be true")
	}

	// 3. Verify GetCover returns exact data and MIME
	rawCover, mime, err := cat.GetCover(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetCover failed: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("expected image/png, got %s", mime)
	}
	if !bytes.Equal(rawCover, coverData) {
		t.Fatal("retrieved cover bytes mismatch")
	}

	// 4. Non-existent track cover returns ErrTrackNotFound
	_, _, err = cat.GetCover(ctx, "nonexistent-id")
	if err == nil {
		t.Fatal("expected ErrTrackNotFound for nonexistent cover, got nil")
	}
}


func TestCatalogTrackLyrics(t *testing.T) {
	ctx := context.Background()
	cat, imp, _ := setupTestCatalog(t)
	_, privKey, _ := crypto.GenerateKeyPair()

	audioData := make([]byte, 50*1024)
	initialLyrics := "[00:01.50] Hello ResonaMesh\n[00:05.00] Decentralized audio stream"

	m, err := imp.Import(ctx, importer.ImportParams{
		Title:        "Lyrics Track",
		Artist:       "Poet",
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(audioData),
		Lyrics:       initialLyrics,
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// 1. Verify GetTrack has initial lyrics
	track, err := cat.GetTrack(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetTrack failed: %v", err)
	}
	if track.Lyrics != initialLyrics {
		t.Fatalf("expected lyrics %q, got %q", initialLyrics, track.Lyrics)
	}

	// 2. Verify GetManifest contains lyrics
	manifest, err := cat.GetManifest(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetManifest failed: %v", err)
	}
	if manifest.Lyrics != initialLyrics {
		t.Fatalf("expected manifest lyrics %q, got %q", initialLyrics, manifest.Lyrics)
	}

	// 3. Update lyrics via UpdateTrackLyrics
	updatedLyrics := "[00:02.00] Synchronized line updated\n[00:07.00] Karaoke mode active"
	if err := cat.UpdateTrackLyrics(ctx, m.ContentID, updatedLyrics); err != nil {
		t.Fatalf("UpdateTrackLyrics failed: %v", err)
	}

	trackAfterUpdate, err := cat.GetTrack(ctx, m.ContentID)
	if err != nil {
		t.Fatalf("GetTrack after update failed: %v", err)
	}
	if trackAfterUpdate.Lyrics != updatedLyrics {
		t.Fatalf("expected updated lyrics %q, got %q", updatedLyrics, trackAfterUpdate.Lyrics)
	}

	// 4. Update lyrics on non-existent track returns ErrTrackNotFound
	if err := cat.UpdateTrackLyrics(ctx, "nonexistent-id", updatedLyrics); !errors.Is(err, catalog.ErrTrackNotFound) {
		t.Fatalf("expected ErrTrackNotFound, got %v", err)
	}
}
