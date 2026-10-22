package player_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"resonamesh/internal/catalog"
	"resonamesh/internal/importer"
	"resonamesh/internal/player"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func setupTestAudio(t *testing.T, size int) (*player.Streamer, string, []byte) {
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

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	streamer := player.NewStreamer(cat, bs)

	// Generate deterministic audio buffer
	audioData := make([]byte, size)
	for i := range audioData {
		audioData[i] = byte((i * 31) % 256)
	}

	_, privKey, _ := crypto.GenerateKeyPair()
	manifest, err := imp.Import(context.Background(), importer.ImportParams{
		Title:        "Streaming Test Track",
		Artist:       "Audio Engineer",
		DurationMs:   200000,
		BitrateKbps:  192,
		LicenseCode:  protocol.LicenseCC0,
		PublisherKey: privKey,
		AudioData:    bytes.NewReader(audioData),
	})
	if err != nil {
		t.Fatalf("failed to import test audio: %v", err)
	}

	return streamer, manifest.ContentID, audioData
}

func TestStreamerCrossChunkRange(t *testing.T) {
	// 600 KiB = 614,400 bytes (chunk 0: [0..262143], chunk 1: [262144..524287], chunk 2: [524288..614399])
	const size = 600 * 1024
	streamer, contentID, originalBytes := setupTestAudio(t, size)

	// Range from byte 200,000 to 400,000 spans across chunk 0 and chunk 1
	const start = 200000
	const end = 400000

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/stream/%s", contentID), nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	rec := httptest.NewRecorder()

	streamer.StreamTrack(rec, req, contentID)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
	}

	expectedCR := fmt.Sprintf("bytes %d-%d/%d", start, end, size)
	if res.Header.Get("Content-Range") != expectedCR {
		t.Fatalf("expected Content-Range %s, got %s", expectedCR, res.Header.Get("Content-Range"))
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	expectedLen := end - start + 1
	if len(body) != expectedLen {
		t.Fatalf("expected payload length %d, got %d", expectedLen, len(body))
	}

	// Verify byte-for-byte fidelity across the chunk boundary
	if !bytes.Equal(body, originalBytes[start:end+1]) {
		t.Fatal("audio payload across chunk boundary does not match original bytes")
	}
}

func TestStreamerOpenEndedRange(t *testing.T) {
	const size = 600 * 1024
	streamer, contentID, originalBytes := setupTestAudio(t, size)

	// From byte 500,000 to the very end (spans chunk 1 and chunk 2)
	const start = 500000
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	rec := httptest.NewRecorder()

	streamer.StreamTrack(rec, req, contentID)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	expectedBytes := originalBytes[start:]
	if !bytes.Equal(body, expectedBytes) {
		t.Fatal("open-ended range bytes mismatch")
	}
}

func TestStreamerSuffixRange(t *testing.T) {
	const size = 600 * 1024
	streamer, contentID, originalBytes := setupTestAudio(t, size)

	// Last 1024 bytes
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	req.Header.Set("Range", "bytes=-1024")
	rec := httptest.NewRecorder()

	streamer.StreamTrack(rec, req, contentID)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	expectedBytes := originalBytes[size-1024:]
	if !bytes.Equal(body, expectedBytes) {
		t.Fatal("suffix range bytes mismatch")
	}
}

func TestStreamerFullGETAndHEAD(t *testing.T) {
	const size = 300 * 1024
	streamer, contentID, originalBytes := setupTestAudio(t, size)

	// 1. Full GET
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	streamer.StreamTrack(rec, req, contentID)
	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for full GET, got %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !bytes.Equal(body, originalBytes) {
		t.Fatal("full GET bytes mismatch")
	}

	// 2. HEAD Request
	headReq := httptest.NewRequest(http.MethodHead, "/stream", nil)
	headRec := httptest.NewRecorder()
	streamer.StreamTrack(headRec, headReq, contentID)
	headRes := headRec.Result()
	defer headRes.Body.Close()

	if headRes.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD, got %d", headRes.StatusCode)
	}
	headBody, _ := io.ReadAll(headRes.Body)
	if len(headBody) != 0 {
		t.Fatalf("HEAD request must return empty body, got %d bytes", len(headBody))
	}
	if headRes.Header.Get("Content-Length") != fmt.Sprintf("%d", size) {
		t.Fatalf("HEAD Content-Length mismatch: %s", headRes.Header.Get("Content-Length"))
	}
}

func TestStreamerErrors(t *testing.T) {
	const size = 100 * 1024
	streamer, contentID, _ := setupTestAudio(t, size)

	// 1. Track not found
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()
	streamer.StreamTrack(rec, req, "non-existent-content-id")
	if rec.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown track, got %d", rec.Result().StatusCode)
	}

	// 2. Out of bounds range
	rangeReq := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rangeReq.Header.Set("Range", "bytes=500000-600000")
	rangeRec := httptest.NewRecorder()
	streamer.StreamTrack(rangeRec, rangeReq, contentID)
	if rangeRec.Result().StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 for out-of-bounds range, got %d", rangeRec.Result().StatusCode)
	}
}
