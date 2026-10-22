package api_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/api"
	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/player"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func setupTestServer(t *testing.T) (*httptest.Server, *catalog.Catalog, ed25519.PrivateKey) {
	t.Helper()
	tempDir := t.TempDir()

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open test sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	bs, err := storage.NewBlockStore(tempDir)
	if err != nil {
		t.Fatalf("failed to init blockstore: %v", err)
	}

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	str := player.NewStreamer(cat, bs)

	_, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate node key: %v", err)
	}

	cfg := &config.Config{
		HTTPAddr:          "127.0.0.1:0",
		P2PPort:           4001,
		DataDir:           tempDir,
		StorageQuotaBytes: 10 * 1024 * 1024,
	}

	server := api.NewServer(cfg, cat, imp, str, priv)
	ts := httptest.NewServer(server.Handler())
	t.Cleanup(ts.Close)

	return ts, cat, priv
}


func TestAPINodeStatus(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/node/status")
	if err != nil {
		t.Fatalf("failed GET /api/v1/node/status: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var status map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}

	if status["version"] != "0.1.0-alpha" {
		t.Fatalf("expected version 0.1.0-alpha, got %v", status["version"])
	}
	if status["publisher_key"] == "" {
		t.Fatal("expected publisher_key to be present")
	}
}

func TestAPITrackLifecycleAndStreaming(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	// 1. Prepare 600 KiB synthetic audio payload spanning 3 blocks
	audioData := make([]byte, 600*1024)
	for i := range audioData {
		audioData[i] = byte(i % 251)
	}

	// 2. Import track via multipart/form-data upload
	var reqBody bytes.Buffer
	writer := multipart.NewWriter(&reqBody)

	_ = writer.WriteField("title", "Ой у лузі червона калина")
	_ = writer.WriteField("artist", "Resona Folk Swarm")
	_ = writer.WriteField("description", "Ukrainian Anthem")
	_ = writer.WriteField("license", protocol.LicenseCCBY40)
	_ = writer.WriteField("bitrate_kbps", "320")
	_ = writer.WriteField("duration_ms", "154000")

	part, err := writer.CreateFormFile("audio_file", "kalyna.mp3")
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := part.Write(audioData); err != nil {
		t.Fatalf("failed to write multipart audio: %v", err)
	}
	_ = writer.Close()

	uploadResp, err := http.Post(ts.URL+"/api/v1/tracks/import", writer.FormDataContentType(), &reqBody)
	if err != nil {
		t.Fatalf("failed POST /api/v1/tracks/import: %v", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(uploadResp.Body)
		t.Fatalf("expected 201 Created, got %d: %s", uploadResp.StatusCode, string(body))
	}

	var importResult struct {
		Manifest protocol.Manifest `json:"manifest"`
		Track    catalog.Track     `json:"track"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&importResult); err != nil {
		t.Fatalf("failed to decode import response: %v", err)
	}

	contentID := importResult.Manifest.ContentID
	if contentID == "" {
		t.Fatal("content_id is empty in import response")
	}
	if importResult.Track.Title != "Ой у лузі червона калина" {
		t.Fatalf("unexpected title: %s", importResult.Track.Title)
	}
	if importResult.Track.AvailabilityPct != 100.0 {
		t.Fatalf("expected 100%% availability, got %f", importResult.Track.AvailabilityPct)
	}

	// 3. List tracks
	listResp, err := http.Get(ts.URL + "/api/v1/tracks")
	if err != nil {
		t.Fatalf("failed GET /api/v1/tracks: %v", err)
	}
	defer listResp.Body.Close()

	var listResult struct {
		Items []catalog.Track `json:"items"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listResult); err != nil {
		t.Fatalf("failed to decode track list: %v", err)
	}
	if listResult.Total != 1 || len(listResult.Items) != 1 {
		t.Fatalf("expected 1 track in list, got total=%d, items=%d", listResult.Total, len(listResult.Items))
	}

	// 4. Search tracks using FTS5 Ukrainian search
	searchResp, err := http.Get(ts.URL + "/api/v1/tracks?q=калина")
	if err != nil {
		t.Fatalf("failed search request: %v", err)
	}
	defer searchResp.Body.Close()

	var searchResult struct {
		Items []catalog.Track `json:"items"`
		Total int             `json:"total"`
	}
	_ = json.NewDecoder(searchResp.Body).Decode(&searchResult)
	if searchResult.Total != 1 {
		t.Fatalf("expected 1 search match for 'калина', got %d", searchResult.Total)
	}

	// 5. Get track details
	getResp, err := http.Get(fmt.Sprintf("%s/api/v1/tracks/%s", ts.URL, contentID))
	if err != nil {
		t.Fatalf("failed GET track: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	// 6. Get manifest
	manifestResp, err := http.Get(fmt.Sprintf("%s/api/v1/tracks/%s/manifest", ts.URL, contentID))
	if err != nil {
		t.Fatalf("failed GET manifest: %v", err)
	}
	defer manifestResp.Body.Close()
	if manifestResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", manifestResp.StatusCode)
	}

	// 7. Pin track
	pinBody := bytes.NewBufferString(`{"pinned": true}`)
	pinResp, err := http.Post(fmt.Sprintf("%s/api/v1/tracks/%s/pin", ts.URL, contentID), "application/json", pinBody)
	if err != nil {
		t.Fatalf("failed POST pin: %v", err)
	}
	defer pinResp.Body.Close()
	if pinResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for pin, got %d", pinResp.StatusCode)
	}

	// 8. Stream audio with byte range request
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/stream/%s", ts.URL, contentID), nil)
	req.Header.Set("Range", "bytes=100-200")

	client := &http.Client{}
	streamResp, err := client.Do(req)
	if err != nil {
		t.Fatalf("failed range request: %v", err)
	}
	defer streamResp.Body.Close()

	if streamResp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 Partial Content, got %d", streamResp.StatusCode)
	}
	streamedBytes, err := io.ReadAll(streamResp.Body)
	if err != nil {
		t.Fatalf("failed to read range response body: %v", err)
	}
	if !bytes.Equal(streamedBytes, audioData[100:201]) {
		t.Fatal("streamed range bytes do not match expected chunk data")
	}

	// 9. CORS preflight OPTIONS request
	optReq, _ := http.NewRequest(http.MethodOptions, ts.URL+"/api/v1/tracks", nil)
	optResp, err := client.Do(optReq)
	if err != nil {
		t.Fatalf("failed OPTIONS request: %v", err)
	}
	defer optResp.Body.Close()
	if optResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204 No Content for OPTIONS, got %d", optResp.StatusCode)
	}
	if optResp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("expected Access-Control-Allow-Origin header")
	}

	// 10. Delete track
	delReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/tracks/%s", ts.URL, contentID), nil)
	delResp, err := client.Do(delReq)
	if err != nil {
		t.Fatalf("failed DELETE track: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", delResp.StatusCode)
	}

	// 11. Verify 404 after deletion
	getDeletedResp, err := http.Get(fmt.Sprintf("%s/api/v1/tracks/%s", ts.URL, contentID))
	if err != nil {
		t.Fatalf("failed GET deleted track: %v", err)
	}
	defer getDeletedResp.Body.Close()
	if getDeletedResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for deleted track, got %d", getDeletedResp.StatusCode)
	}
}

func TestAPIErrorHandlingAndValidation(t *testing.T) {
	ts, _, _ := setupTestServer(t)
	client := &http.Client{}

	// Missing audio_file in import
	var emptyBody bytes.Buffer
	w := multipart.NewWriter(&emptyBody)
	_ = w.WriteField("title", "Test Track")
	_ = w.WriteField("artist", "Test Artist")
	_ = w.Close()

	resp, err := http.Post(ts.URL+"/api/v1/tracks/import", w.FormDataContentType(), &emptyBody)
	if err != nil {
		t.Fatalf("failed post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing audio_file, got %d", resp.StatusCode)
	}

	// Missing title in import
	var missingTitleBody bytes.Buffer
	w2 := multipart.NewWriter(&missingTitleBody)
	_ = w2.WriteField("artist", "Test Artist")
	part, _ := w2.CreateFormFile("audio_file", "test.mp3")
	_, _ = part.Write([]byte("fake-audio-bytes"))
	_ = w2.Close()

	resp2, err := http.Post(ts.URL+"/api/v1/tracks/import", w2.FormDataContentType(), &missingTitleBody)
	if err != nil {
		t.Fatalf("failed post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing title, got %d", resp2.StatusCode)
	}

	// Invalid license
	var badLicenseBody bytes.Buffer
	w3 := multipart.NewWriter(&badLicenseBody)
	_ = w3.WriteField("title", "Test Title")
	_ = w3.WriteField("artist", "Test Artist")
	_ = w3.WriteField("license", "GPL-3.0")
	part3, _ := w3.CreateFormFile("audio_file", "test.mp3")
	_, _ = part3.Write([]byte("fake-audio-bytes"))
	_ = w3.Close()

	resp3, err := http.Post(ts.URL+"/api/v1/tracks/import", w3.FormDataContentType(), &badLicenseBody)
	if err != nil {
		t.Fatalf("failed post: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad license, got %d", resp3.StatusCode)
	}

	// Nonexistent track 404s
	resp4, err := http.Get(ts.URL + "/api/v1/tracks/nonexistent-content-id")
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent track, got %d", resp4.StatusCode)
	}

	resp5, err := http.Get(ts.URL + "/api/v1/tracks/nonexistent-content-id/manifest")
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for nonexistent manifest, got %d", resp5.StatusCode)
	}

	// Pin invalid JSON
	resp6, err := http.Post(ts.URL+"/api/v1/tracks/some-id/pin", "application/json", bytes.NewBufferString("not-json"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed json pin, got %d", resp6.StatusCode)
	}

	// Pin nonexistent track
	resp7, err := http.Post(ts.URL+"/api/v1/tracks/nonexistent-id/pin", "application/json", bytes.NewBufferString(`{"pinned":true}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp7.Body.Close()
	if resp7.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for pin on nonexistent track, got %d", resp7.StatusCode)
	}

	// Delete nonexistent track
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/tracks/nonexistent-id", nil)
	resp8, err := client.Do(delReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp8.Body.Close()
	if resp8.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for delete on nonexistent track, got %d", resp8.StatusCode)
	}

	// Stream nonexistent track
	resp9, err := http.Get(ts.URL + "/api/v1/stream/nonexistent-id")
	if err != nil {
		t.Fatal(err)
	}
	defer resp9.Body.Close()
	if resp9.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for stream on nonexistent track, got %d", resp9.StatusCode)
	}
}

func TestAPIServerLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bs, err := storage.NewBlockStore(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	cat := catalog.New(db, bs)
	imp := importer.NewImporter(bs, db)
	str := player.NewStreamer(cat, bs)
	_, priv, _ := crypto.GenerateKeyPair()

	cfg := &config.Config{
		HTTPAddr:          "127.0.0.1:0",
		P2PPort:           4001,
		DataDir:           tempDir,
		StorageQuotaBytes: 10 * 1024 * 1024,
	}

	server := api.NewServer(cfg, cat, imp, str, priv)

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.Start()
	}()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("failed to shutdown server: %v", err)
	}

	if err := <-errChan; err != nil {
		t.Fatalf("server start returned unexpected error: %v", err)
	}
}


