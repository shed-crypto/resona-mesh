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
	"strings"
	"testing"
	"time"

	"resonamesh/internal/api"
	"resonamesh/internal/catalog"
	"resonamesh/internal/config"
	"resonamesh/internal/importer"
	"resonamesh/internal/moderation"
	"resonamesh/internal/p2p"
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

	modEngine, err := moderation.NewEngine(db, moderation.Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: crypto.PublicKeyToHex(priv.Public().(ed25519.PublicKey)),
	}, priv)
	if err == nil {
		server.SetModerationEngine(modEngine, priv)
	}

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

func TestAPIEmbeddedFrontend(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("failed GET /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for root embedded frontend, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "ResonaMesh") {
		t.Fatalf("expected HTML body to contain ResonaMesh title, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "id=\"root\"") {
		t.Fatalf("expected HTML body to contain root element, got: %s", bodyStr)
	}
}

func TestAPIPeersEndpoint(t *testing.T) {
	tempDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tempDir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	bs, _ := storage.NewBlockStore(tempDir)
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
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. Initial empty peers
	resp, err := http.Get(ts.URL + "/api/v1/peers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/peers, got %d", resp.StatusCode)
	}

	var data struct {
		Peers []map[string]interface{} `json:"peers"`
		Total int                      `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 0 {
		t.Fatalf("expected 0 initial peers, got %d", data.Total)
	}

	// 2. Set peer tracker
	tracker := p2p.NewPeerTracker()
	server.SetPeerTracker(tracker)

	resp2, err := http.Get(ts.URL + "/api/v1/peers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if err := json.NewDecoder(resp2.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if data.Total != 0 {
		t.Fatalf("expected 0 tracked peers, got %d", data.Total)
	}
}

func createTestAPIServer(t *testing.T) (*api.Server, string) {
	t.Helper()
	tempDir := t.TempDir()
	db, err := storage.Open(filepath.Join(tempDir, "catalog.db"))
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
	return server, tempDir
}

func TestStorageAndSettingsEndpoints(t *testing.T) {
	server, tmpDir := createTestAPIServer(t)
	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()
	bs, _ := storage.NewBlockStore(tmpDir)
	qm := storage.NewQuotaManager(5*1024*1024, tmpDir, db, bs, nil)
	server.SetQuotaManager(qm)

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. GET /api/v1/storage
	resp, err := http.Get(ts.URL + "/api/v1/storage")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/storage, got %d", resp.StatusCode)
	}

	var usage storage.UsageStats
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		t.Fatal(err)
	}
	if usage.QuotaBytes != 5*1024*1024 {
		t.Fatalf("expected quota 5 MiB, got %d", usage.QuotaBytes)
	}

	// 2. GET /api/v1/settings
	resp2, err := http.Get(ts.URL + "/api/v1/settings")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/settings, got %d", resp2.StatusCode)
	}

	// 3. PUT /api/v1/settings
	newQuota := int64(8 * 1024 * 1024)
	newKeeper := true
	reqBody, _ := json.Marshal(map[string]interface{}{
		"storage_quota_bytes": newQuota,
		"keeper_enabled":      newKeeper,
	})

	req, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/v1/settings", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for PUT /api/v1/settings, got %d", putResp.StatusCode)
	}

	if qm.QuotaBytes() != newQuota {
		t.Fatalf("expected quota updated in qm to %d, got %d", newQuota, qm.QuotaBytes())
	}
}

func TestPinAndUnpinEndpoints(t *testing.T) {
	server, _ := createTestAPIServer(t)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// 1. Import a track first
	sampleAudio := make([]byte, 100*1024)

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("title", "Pin Test Track")
	_ = writer.WriteField("artist", "Sound Artist")
	_ = writer.WriteField("license_code", protocol.LicenseCC0)
	part, _ := writer.CreateFormFile("audio_file", "sample.mp3")
	_, _ = part.Write(sampleAudio)
	_ = writer.Close()

	uploadResp, err := http.Post(ts.URL+"/api/v1/tracks/import", writer.FormDataContentType(), &buf)
	if err != nil || uploadResp.StatusCode != http.StatusCreated {
		t.Fatalf("import failed: status %d (err: %v)", uploadResp.StatusCode, err)
	}
	var created struct {
		ContentID string `json:"content_id"`
	}
	_ = json.NewDecoder(uploadResp.Body).Decode(&created)
	uploadResp.Body.Close()

	// 2. Pin track via POST /api/v1/tracks/{id}/pin
	pinBody, _ := json.Marshal(map[string]bool{"pinned": true})
	pinResp, err := http.Post(ts.URL+"/api/v1/tracks/"+created.ContentID+"/pin", "application/json", bytes.NewReader(pinBody))
	if err != nil || pinResp.StatusCode != http.StatusOK {
		t.Fatalf("pin failed: status %d (err: %v)", pinResp.StatusCode, err)
	}
	pinResp.Body.Close()

	// 3. Unpin track via POST /api/v1/tracks/{id}/unpin
	unpinResp, err := http.Post(ts.URL+"/api/v1/tracks/"+created.ContentID+"/unpin", "application/json", nil)
	if err != nil || unpinResp.StatusCode != http.StatusOK {
		t.Fatalf("unpin failed: status %d (err: %v)", unpinResp.StatusCode, err)
	}
	var unpinResult struct {
		Pinned bool `json:"pinned"`
	}
	_ = json.NewDecoder(unpinResp.Body).Decode(&unpinResult)
	unpinResp.Body.Close()

	if unpinResult.Pinned {
		t.Fatal("expected track pinned to be false after unpin")
	}
}

func TestManualPeerConnectEndpoint(t *testing.T) {
	server, _ := createTestAPIServer(t)

	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

	hostA, _ := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	hostB, _ := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	server.SetP2PHost(hostA)
	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	// Connect using Host B's multiaddress
	bAddrs := hostB.Addrs()
	if len(bAddrs) == 0 {
		t.Fatal("host B has no listen addrs")
	}
	bMultiaddr := bAddrs[0].String() + "/p2p/" + hostB.ID().String()

	body, _ := json.Marshal(map[string]string{
		"addr": bMultiaddr,
	})

	resp, err := http.Post(ts.URL+"/api/v1/peers/connect", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for manual peer connect, got %d", resp.StatusCode)
	}

	var res struct {
		Status string `json:"status"`
		PeerID string `json:"peer_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "connected" || res.PeerID != hostB.ID().String() {
		t.Fatalf("unexpected connect result: %+v", res)
	}
}

func TestAPIModerationEndpoints(t *testing.T) {
	ts, _, _ := setupTestServer(t)

	// 1. Submit a report
	repPayload := map[string]string{
		"content_id":         "reported-content-99",
		"reason_code":        protocol.ReasonCopyrightClaim,
		"description":        "Infringing copyright claims",
		"evidence_reference": "https://example.com/claim",
	}
	repBytes, _ := json.Marshal(repPayload)

	resp, err := http.Post(ts.URL+"/api/v1/reports", "application/json", bytes.NewReader(repBytes))
	if err != nil {
		t.Fatalf("failed to post report: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for report submission, got %d", resp.StatusCode)
	}

	var createdReport protocol.ReportEvent
	if err := json.NewDecoder(resp.Body).Decode(&createdReport); err != nil {
		t.Fatalf("failed to parse created report: %v", err)
	}
	if createdReport.ContentID != "reported-content-99" || createdReport.Signature == "" {
		t.Fatalf("unexpected report structure: %+v", createdReport)
	}

	// 2. List reports
	respList, err := http.Get(ts.URL + "/api/v1/reports")
	if err != nil {
		t.Fatalf("failed to get reports: %v", err)
	}
	defer respList.Body.Close()

	if respList.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for list reports, got %d", respList.StatusCode)
	}

	var reportList struct {
		Items []moderation.ReportRecord `json:"items"`
		Total int                       `json:"total"`
	}
	if err := json.NewDecoder(respList.Body).Decode(&reportList); err != nil {
		t.Fatalf("failed to parse report list: %v", err)
	}
	if reportList.Total != 1 || len(reportList.Items) != 1 {
		t.Fatalf("expected 1 report in list, got total=%d", reportList.Total)
	}

	// 3. Create a moderation decision (BLOCK)
	decPayload := map[string]string{
		"content_id":  "reported-content-99",
		"action":      protocol.ActionBlock,
		"reason_code": protocol.ReasonCopyrightClaim,
		"reason_text": "Approved copyright takedown",
	}
	decBytes, _ := json.Marshal(decPayload)

	respDec, err := http.Post(ts.URL+"/api/v1/moderation/decisions", "application/json", bytes.NewReader(decBytes))
	if err != nil {
		t.Fatalf("failed to post decision: %v", err)
	}
	defer respDec.Body.Close()

	if respDec.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for decision, got %d", respDec.StatusCode)
	}

	// 4. List decisions
	respDecList, err := http.Get(ts.URL + "/api/v1/moderation/decisions")
	if err != nil {
		t.Fatalf("failed to get decisions: %v", err)
	}
	defer respDecList.Body.Close()

	var decList struct {
		Items []moderation.DecisionRecord `json:"items"`
		Total int                         `json:"total"`
	}
	if err := json.NewDecoder(respDecList.Body).Decode(&decList); err != nil {
		t.Fatalf("failed to parse decision list: %v", err)
	}
	if decList.Total != 1 || len(decList.Items) != 1 {
		t.Fatalf("expected 1 decision in list, got total=%d", decList.Total)
	}

	// 5. List blocked items
	respBlocked, err := http.Get(ts.URL + "/api/v1/moderation/blocked")
	if err != nil {
		t.Fatalf("failed to get blocked items: %v", err)
	}
	defer respBlocked.Body.Close()

	var blockedList struct {
		Items []moderation.BlockedRecord `json:"items"`
		Total int                        `json:"total"`
	}
	if err := json.NewDecoder(respBlocked.Body).Decode(&blockedList); err != nil {
		t.Fatalf("failed to parse blocked list: %v", err)
	}
	if blockedList.Total != 1 || len(blockedList.Items) != 1 {
		t.Fatalf("expected 1 blocked item, got total=%d", blockedList.Total)
	}
	if blockedList.Items[0].ContentID != "reported-content-99" {
		t.Fatalf("unexpected blocked content ID: %s", blockedList.Items[0].ContentID)
	}
}




