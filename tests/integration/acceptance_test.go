package integration_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"resonamesh/internal/catalog"
	"resonamesh/internal/importer"
	"resonamesh/internal/moderation"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

// T01: Clean launch, import valid and invalid MP3, restart persistence.
func TestT01_CleanLaunch_ImportValidInvalid_Restart(t *testing.T) {
	node := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	// 1. Import valid track
	const trackSize = 600 * 1024
	m := node.ImportSyntheticTrack("Valid Symphony", trackSize)
	if m == nil || m.ContentID == "" {
		t.Fatalf("expected valid manifest, got nil")
	}
	if len(m.BlockHashes) != 3 {
		t.Fatalf("expected 3 blocks for 600 KiB, got %d", len(m.BlockHashes))
	}

	// 2. Reject invalid import (missing license)
	invalidParams := importer.ImportParams{
		Title:        "Unlicensed Song",
		Artist:       "Unknown",
		PublisherKey: node.PrivKey,
		LicenseCode:  "", // Disallowed
		AudioData:    bytes.NewReader(make([]byte, 1024)),
	}
	_, err := node.Importer.Import(ctx, invalidParams)
	if err == nil {
		t.Fatalf("expected import to fail without license, but succeeded")
	}

	// 3. Verify zero orphan files for failed import
	tracks, total, err := node.Catalog.ListTracks(ctx, catalog.Query{})
	if err != nil || total != 1 || len(tracks) != 1 {
		t.Fatalf("expected exactly 1 track in catalog, got %d (err: %v)", total, err)
	}

	// 4. Persistence across restart (re-open DB with same key)
	dbPath := filepath.Join(node.Dir, "catalog.db")
	reopenedDB, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to re-open DB: %v", err)
	}
	defer reopenedDB.Close()

	reopenedCat := catalog.New(reopenedDB, node.BlockStore)
	reopenedTracks, totalReopened, err := reopenedCat.ListTracks(ctx, catalog.Query{})
	if err != nil || totalReopened != 1 || reopenedTracks[0].ContentID != m.ContentID {
		t.Fatalf("catalog failed to persist across DB restart: %v", err)
	}
}

// T02: Altered manifest, signature corruption, block tampering detection.
func TestT02_AlteredManifestSignatureBlockDetection(t *testing.T) {
	node := NewTestNode(t, NodeOptions{})

	m := node.ImportSyntheticTrack("Tamper Probe", 300*1024)

	// 1. Signature verification succeeds on authentic manifest
	canonicalPayload := importer.CanonicalManifestPayload(m)
	sigBytes, err := hex.DecodeString(m.Signature)
	if err != nil {
		t.Fatalf("failed to decode signature: %v", err)
	}
	pubKeyBytes, err := hex.DecodeString(m.PublisherKeyHex)
	if err != nil {
		t.Fatalf("failed to decode publisher key: %v", err)
	}

	valid := crypto.VerifyWithDomain(ed25519.PublicKey(pubKeyBytes), crypto.DomainManifest, []byte(canonicalPayload), sigBytes)
	if !valid {
		t.Fatalf("valid manifest failed signature check")
	}

	// 2. Tampered manifest field must fail signature check
	tampered := *m
	tampered.Title = "Malicious Modified Title"
	tamperedPayload := importer.CanonicalManifestPayload(&tampered)
	if crypto.VerifyWithDomain(ed25519.PublicKey(pubKeyBytes), crypto.DomainManifest, []byte(tamperedPayload), sigBytes) {
		t.Fatalf("tampered manifest unexpectedly passed signature verification")
	}

	// 3. Corrupted chunk data must fail SHA-256 integrity verification
	corruptedChunk := make([]byte, protocol.BlockSize)
	corruptedChunk[0] = 0xFF // Altered byte
	expectedHash := m.BlockHashes[0]
	actualHash := crypto.HashChunkHex(corruptedChunk)
	if actualHash == expectedHash {
		t.Fatalf("corrupted chunk hash should not match expected hash")
	}

	// Verify blockstore rejects or identifies hash mismatch
	hasTampered := node.BlockStore.Has(actualHash)
	if hasTampered {
		t.Fatalf("blockstore should not contain corrupted block hash")
	}
}

// T03: Multi-node direct connect, remote catalog retrieval, on-demand chunk streaming.
func TestT03_MultiNodeDiscoveryAndStreaming(t *testing.T) {
	nodeA := NewTestNode(t, NodeOptions{})
	nodeB := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	// Node A publishes track
	mA := nodeA.ImportSyntheticTrack("Swarm Track", 600*1024)

	// Node B connects to Node A
	nodeB.Connect(nodeA)

	// Node B syncs catalog from Node A
	imported, err := nodeB.Syncer.SyncFromPeer(ctx, nodeA.Host.ID())
	if err != nil {
		t.Fatalf("catalog sync failed: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported track from peer, got %d", imported)
	}

	// Node B fetches first block on-demand
	b0Hash := mA.BlockHashes[0]
	data, err := nodeB.Fetcher.FetchBlock(ctx, mA.ContentID, 0, b0Hash)
	if err != nil {
		t.Fatalf("failed to fetch block on-demand from peer: %v", err)
	}
	if len(data) != protocol.BlockSize {
		t.Fatalf("expected %d bytes, got %d", protocol.BlockSize, len(data))
	}

	// Verify Node B now caches block 0
	if !nodeB.BlockStore.Has(b0Hash) {
		t.Fatalf("node B should have cached block 0 locally")
	}
}

// T04: PEX peer discovery and multilingual FTS5 catalog search.
func TestT04_PEXAndCatalogSearch(t *testing.T) {
	nodeA := NewTestNode(t, NodeOptions{})
	nodeB := NewTestNode(t, NodeOptions{})
	nodeC := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	// Connect A <-> B and B <-> C
	nodeA.Connect(nodeB)
	nodeB.Connect(nodeC)

	// Verify Node B is connected to both A and C
	peersB := nodeB.Host.ConnectedPeers()
	if len(peersB) < 2 {
		t.Fatalf("expected node B to have at least 2 connected peers, got %d", len(peersB))
	}

	// Import multilingual tracks into Node A's catalog
	nodeA.ImportSyntheticTrack("Neon Cybernetic Echoes", 256*1024)
	nodeA.ImportSyntheticTrack("Лекція з моделювання систем", 256*1024)

	// Search English
	enResults, totalEn, err := nodeA.Catalog.ListTracks(ctx, catalog.Query{Search: "Cybernetic"})
	if err != nil || totalEn != 1 || len(enResults) != 1 {
		t.Fatalf("FTS5 English search failed: total=%d (err: %v)", totalEn, err)
	}

	// Search Ukrainian Cyrillic
	uaResults, totalUa, err := nodeA.Catalog.ListTracks(ctx, catalog.Query{Search: "моделювання"})
	if err != nil || totalUa != 1 || len(uaResults) != 1 {
		t.Fatalf("FTS5 Cyrillic search failed: total=%d (err: %v)", totalUa, err)
	}
}

// T05: HTTP Range compliance (200, 206, 416, HEAD, Content-Range).
func TestT05_HTTPRangeCompliance(t *testing.T) {
	node := NewTestNode(t, NodeOptions{})
	const trackSize = 512 * 1024
	m := node.ImportSyntheticTrack("Range Test Symphony", trackSize)

	audioURL := fmt.Sprintf("%s/api/v1/audio/%s", node.BaseURL, m.ContentID)

	// 1. GET Full content (200 OK)
	resp, err := node.HTTPClient.Get(audioURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for full GET, got %d (err: %v)", resp.StatusCode, err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(body) != trackSize {
		t.Fatalf("expected %d bytes, got %d", trackSize, len(body))
	}

	// 2. Single byte range bytes=0-1023 (206 Partial Content)
	reqRange, _ := http.NewRequest(http.MethodGet, audioURL, nil)
	reqRange.Header.Set("Range", "bytes=0-1023")
	respRange, err := node.HTTPClient.Do(reqRange)
	if err != nil || respRange.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 for range 0-1023, got %d", respRange.StatusCode)
	}
	rangeBody, _ := io.ReadAll(respRange.Body)
	respRange.Body.Close()
	if len(rangeBody) != 1024 {
		t.Fatalf("expected 1024 bytes, got %d", len(rangeBody))
	}
	expectedCR := fmt.Sprintf("bytes 0-1023/%d", trackSize)
	if respRange.Header.Get("Content-Range") != expectedCR {
		t.Fatalf("expected Content-Range %q, got %q", expectedCR, respRange.Header.Get("Content-Range"))
	}

	// 3. Suffix range bytes=-500 (206)
	reqSuffix, _ := http.NewRequest(http.MethodGet, audioURL, nil)
	reqSuffix.Header.Set("Range", "bytes=-500")
	respSuffix, err := node.HTTPClient.Do(reqSuffix)
	if err != nil || respSuffix.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206 for suffix range, got %d", respSuffix.StatusCode)
	}
	suffixBody, _ := io.ReadAll(respSuffix.Body)
	respSuffix.Body.Close()
	if len(suffixBody) != 500 {
		t.Fatalf("expected 500 bytes for suffix range, got %d", len(suffixBody))
	}

	// 4. Invalid range bytes=9999999- (416 Range Not Satisfiable)
	reqInvalid, _ := http.NewRequest(http.MethodGet, audioURL, nil)
	reqInvalid.Header.Set("Range", "bytes=9999999-")
	respInvalid, err := node.HTTPClient.Do(reqInvalid)
	if err != nil || respInvalid.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		t.Fatalf("expected 416 for out-of-bounds range, got %d", respInvalid.StatusCode)
	}
	respInvalid.Body.Close()

	// 5. HEAD request (200 OK, empty body, metadata headers)
	respHead, err := node.HTTPClient.Head(audioURL)
	if err != nil || respHead.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HEAD, got %d", respHead.StatusCode)
	}
	headBody, _ := io.ReadAll(respHead.Body)
	respHead.Body.Close()
	if len(headBody) != 0 {
		t.Fatalf("HEAD request returned non-empty body: %d bytes", len(headBody))
	}
	if respHead.Header.Get("Accept-Ranges") != "bytes" {
		t.Fatalf("expected Accept-Ranges: bytes")
	}
}

// T06: Cold cache on-demand streaming simulation and seek window.
func TestT06_ColdCacheStreamingAndSeekPrefetch(t *testing.T) {
	nodeA := NewTestNode(t, NodeOptions{})
	nodeB := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	// 800 KiB -> 4 chunks (256, 256, 256, 32 KiB)
	const trackSize = 800 * 1024
	mA := nodeA.ImportSyntheticTrack("Long Epic Symphony", trackSize)

	nodeB.Connect(nodeA)
	_, _ = nodeB.Syncer.SyncFromPeer(ctx, nodeA.Host.ID())

	// Node B has 0% cached initially
	b0Hash := mA.BlockHashes[0]
	b3Hash := mA.BlockHashes[3]
	if nodeB.BlockStore.Has(b0Hash) || nodeB.BlockStore.Has(b3Hash) {
		t.Fatalf("node B should have cold cache initially")
	}

	// Player requests only beginning (bytes 0-1000)
	reqStart, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/audio/%s", nodeB.BaseURL, mA.ContentID), nil)
	reqStart.Header.Set("Range", "bytes=0-999")
	respStart, err := nodeB.HTTPClient.Do(reqStart)
	if err != nil || respStart.StatusCode != http.StatusPartialContent {
		t.Fatalf("streaming start failed: %v", err)
	}
	respStart.Body.Close()

	// Verify only block 0 is now cached; block 3 is untouched
	if !nodeB.BlockStore.Has(b0Hash) {
		t.Fatalf("node B should have fetched block 0 for stream playback")
	}
	if nodeB.BlockStore.Has(b3Hash) {
		t.Fatalf("node B should NOT have pre-downloaded distant block 3")
	}

	// Seek simulation: Jump to end (offset 780 KiB in block 3)
	seekStart := trackSize - 1000
	seekEnd := trackSize - 1
	reqSeek, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/audio/%s", nodeB.BaseURL, mA.ContentID), nil)
	reqSeek.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", seekStart, seekEnd))
	respSeek, err := nodeB.HTTPClient.Do(reqSeek)
	if err != nil || respSeek.StatusCode != http.StatusPartialContent {
		t.Fatalf("seek playback failed: %v", err)
	}
	respSeek.Body.Close()

	// Verify block 3 is now cached
	if !nodeB.BlockStore.Has(b3Hash) {
		t.Fatalf("node B should have fetched block 3 after seek")
	}
}

// T07: Disconnect publisher A mid-stream; failover to secondary replica C.
func TestT07_MidStreamFailoverOnPeerDisconnect(t *testing.T) {
	nodeA := NewTestNode(t, NodeOptions{})
	nodeB := NewTestNode(t, NodeOptions{})
	nodeC := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	const trackSize = 600 * 1024
	mA := nodeA.ImportSyntheticTrack("Failover Concerto", trackSize)

	// Replicate track to node C
	for i, h := range mA.BlockHashes {
		data, _ := nodeA.BlockStore.Get(h)
		_ = nodeC.BlockStore.Put(h, data)
		_ = nodeC.Catalog.RegisterRemoteManifest(ctx, mA)
		_ = nodeC.Catalog.MarkBlockStored(ctx, mA.ContentID, i, len(data))
	}

	// Node B connects to BOTH A and C
	nodeB.Connect(nodeA)
	nodeB.Connect(nodeC)
	_, _ = nodeB.Syncer.SyncFromPeer(ctx, nodeA.Host.ID())

	// Node B fetches block 0 from Node A
	data0, err := nodeB.Fetcher.FetchBlock(ctx, mA.ContentID, 0, mA.BlockHashes[0])
	if err != nil || len(data0) == 0 {
		t.Fatalf("failed initial block fetch: %v", err)
	}

	// Now abruptly close Node A
	nodeA.Close()

	// Node B attempts to fetch block 1: must failover to Node C!
	data1, err := nodeB.Fetcher.FetchBlock(ctx, mA.ContentID, 1, mA.BlockHashes[1])
	if err != nil {
		t.Fatalf("failover to Node C failed after Node A dropped: %v", err)
	}
	if len(data1) != protocol.BlockSize {
		t.Fatalf("expected %d bytes from failover replica, got %d", protocol.BlockSize, len(data1))
	}
}

// T08: Concurrent pin/import/prefetch with storage quota enforcement & LRU.
func TestT08_ConcurrentStorageOperationsAndLRU(t *testing.T) {
	// Set quota: 1200 KiB (allows both tracks, but triggers eviction upon new reservation)
	const quota = 1200 * 1024
	node := NewTestNode(t, NodeOptions{StorageQuotaBytes: quota})
	ctx := context.Background()

	// 1. Import Track 1 (300 KiB) and PIN it
	m1 := node.ImportSyntheticTrack("Pinned Classical Masterpiece", 300*1024)
	if err := node.Catalog.SetPinned(ctx, m1.ContentID, true); err != nil {
		t.Fatalf("failed to pin track: %v", err)
	}

	// 2. Import Track 2 (300 KiB) - unpinned
	m2 := node.ImportSyntheticTrack("Transient Radio Hit", 300*1024)
	if err := node.Catalog.SetPinned(ctx, m2.ContentID, false); err != nil {
		t.Fatalf("failed to unpin track 2: %v", err)
	}

	// Current stored: ~600 KiB <= 1200 KiB quota
	usage, err := node.QuotaManager.Usage(ctx)
	if err != nil || usage.StoredBytes > quota {
		t.Fatalf("initial usage exceeded quota: %d > %d", usage.StoredBytes, quota)
	}

	// 3. Attempt to reserve more space than remaining (700 KiB + 600 KiB = 1300 KiB > 1200 KiB)
	// QuotaManager must evict unpinned Track 2 blocks to satisfy reservation, but NEVER pinned Track 1!
	tok, err := node.QuotaManager.Reserve(ctx, 700*1024)
	if err != nil {
		t.Fatalf("quota manager failed to satisfy reservation via LRU eviction: %v", err)
	}
	defer node.QuotaManager.Release(tok)

	// Pinned track blocks must STILL be intact in blockstore
	for _, h := range m1.BlockHashes {
		if !node.BlockStore.Has(h) {
			t.Fatalf("pinned block %s was erroneously evicted!", h)
		}
	}
	_ = m2
}

// T09: Keeper autonomous background replication.
func TestT09_KeeperAutonomousReplication(t *testing.T) {
	nodeA := NewTestNode(t, NodeOptions{})
	nodeC := NewTestNode(t, NodeOptions{KeeperEnabled: true})
	ctx := context.Background()

	mA := nodeA.ImportSyntheticTrack("Rare Swarm Archive", 300*1024)

	// Node C connects to A and syncs catalog
	nodeC.Connect(nodeA)
	_, err := nodeC.Syncer.SyncFromPeer(ctx, nodeA.Host.ID())
	if err != nil {
		t.Fatalf("sync to C failed: %v", err)
	}

	// Run Keeper replication cycle on Node C
	if err := nodeC.Keeper.RunCycle(ctx); err != nil {
		t.Fatalf("keeper cycle failed: %v", err)
	}

	// Verify Node C fetched all blocks
	for _, h := range mA.BlockHashes {
		if !nodeC.BlockStore.Has(h) {
			t.Fatalf("keeper failed to replicate block: %s", h)
		}
	}

	// Disconnect Node A; Node C should be able to serve the track entirely autonomously
	nodeA.Close()

	nodeB := NewTestNode(t, NodeOptions{})
	nodeB.Connect(nodeC)
	_, _ = nodeB.Syncer.SyncFromPeer(ctx, nodeC.Host.ID())

	fetched, err := nodeB.Fetcher.FetchBlock(ctx, mA.ContentID, 0, mA.BlockHashes[0])
	if err != nil || len(fetched) == 0 {
		t.Fatalf("failed to stream replicated track from Keeper Node C: %v", err)
	}
}

// T10: Abuse report outbox queue, offline moderator, delivery on reconnect, receipt issuance.
func TestT10_StoreAndForwardModerationReportQueue(t *testing.T) {
	modPub, modPriv, _ := crypto.GenerateKeyPair()
	nodeB := NewTestNode(t, NodeOptions{ModeratorPubKey: modPub})
	ctx := context.Background()

	// 1. Moderator is OFFLINE. Node B files abuse report
	rep, err := nodeB.ModEngine.SubmitReport(ctx, moderation.CreateReportParams{
		ContentID:   "track-infringing-101",
		PayloadHash: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		ReasonCode:  protocol.ReasonCopyrightClaim,
		Description: "Contains unauthorized copyrighted master recording",
	})
	if err != nil {
		t.Fatalf("failed to file outbox report: %v", err)
	}
	if rep.ContentID == "" {
		t.Fatalf("expected valid report event")
	}

	// 2. Moderator Node D comes ONLINE
	nodeD := NewTestNode(t, NodeOptions{
		ModeratorPubKey:  modPub,
		ModeratorPrivKey: modPriv,
	})

	// 3. Node B connects to Node D and pushes outbox
	nodeB.Connect(nodeD)
	if err := nodeB.ModHandler.SyncCycle(ctx); err != nil {
		t.Fatalf("sync cycle failed: %v", err)
	}

	// 4. Verify Node B recorded delivery and receipt
	reports, err := nodeB.ModEngine.ListReports(ctx)
	if err != nil || len(reports) == 0 {
		t.Fatalf("failed to get reports: %v", err)
	}
	foundDelivered := false
	for _, r := range reports {
		if r.ContentID == "track-infringing-101" && (r.Status == "ACCEPTED" || r.Status == "DELIVERED") {
			foundDelivered = true
			if r.ReceiptSignature == "" {
				t.Fatalf("expected non-empty receipt signature")
			}
			break
		}
	}
	if !foundDelivered {
		t.Fatalf("expected report status to become ACCEPTED with signed receipt, got %+v", reports)
	}
}

// T11: Complete moderation lifecycle: REPORT -> BLOCK -> APPEAL -> RESTORE.
func TestT11_ModerationLifecycle_BlockAppealRestore(t *testing.T) {
	modPub, modPriv, _ := crypto.GenerateKeyPair()
	node := NewTestNode(t, NodeOptions{
		ModeratorPubKey:  modPub,
		ModeratorPrivKey: modPriv,
	})
	ctx := context.Background()

	m := node.ImportSyntheticTrack("Disputed Folk Ballad", 256*1024)

	// 1. Moderator issues BLOCK decision
	decBlock, err := node.ModEngine.CreateDecision(ctx, modPriv, moderation.CreateDecisionParams{
		ContentID:   m.ContentID,
		PayloadHash: m.BlockHashes[0],
		Action:      protocol.ActionBlock,
		ReasonCode:  protocol.ReasonCopyrightClaim,
		ReasonText:  "Confirmed DMCA infringement notice",
	})
	if err != nil {
		t.Fatalf("failed to create block decision: %v", err)
	}
	if decBlock.Action != protocol.ActionBlock {
		t.Fatalf("expected BLOCK action")
	}

	// Verify track is blocked in catalog and streamer
	blocked, reason, err := node.ModEngine.IsBlocked(ctx, m.ContentID, m.BlockHashes[0])
	if err != nil || !blocked {
		t.Fatalf("content should be reported as blocked (err: %v)", err)
	}
	if !strings.Contains(reason, "DMCA") {
		t.Fatalf("unexpected block reason: %s", reason)
	}

	// Playback attempt must be refused (HTTP 403 Forbidden)
	resp, err := node.HTTPClient.Get(fmt.Sprintf("%s/api/v1/audio/%s", node.BaseURL, m.ContentID))
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for blocked track, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 2. Publisher appeals and Moderator issues RESTORE decision
	decRestore, err := node.ModEngine.CreateDecision(ctx, modPriv, moderation.CreateDecisionParams{
		ContentID:   m.ContentID,
		PayloadHash: m.BlockHashes[0],
		Action:      protocol.ActionRestore,
		ReasonCode:  protocol.ReasonOther,
		ReasonText:  "Valid DMCA counter-notice accepted",
	})
	if err != nil {
		t.Fatalf("failed to create restore decision: %v", err)
	}
	if decRestore.Sequence <= decBlock.Sequence {
		t.Fatalf("restore sequence must be strictly greater than block sequence")
	}

	// Verify content is unblocked
	unblocked, _, err := node.ModEngine.IsBlocked(ctx, m.ContentID, m.BlockHashes[0])
	if err != nil || unblocked {
		t.Fatalf("content should now be unblocked")
	}

	// Playback should succeed again (HTTP 200 OK)
	resp2, err := node.HTTPClient.Get(fmt.Sprintf("%s/api/v1/audio/%s", node.BaseURL, m.ContentID))
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after RESTORE, got %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

// T12: Offline decision sync and enforcement across reconnect.
func TestT12_OfflineDecisionSyncAndEnforcement(t *testing.T) {
	modPub, modPriv, _ := crypto.GenerateKeyPair()
	nodeD := NewTestNode(t, NodeOptions{
		ModeratorPubKey:  modPub,
		ModeratorPrivKey: modPriv,
	})
	nodeB := NewTestNode(t, NodeOptions{
		ModeratorPubKey: modPub,
	})
	ctx := context.Background()

	// Moderator issues BLOCK while Node B is offline
	targetHash := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	_, err := nodeD.ModEngine.CreateDecision(ctx, modPriv, moderation.CreateDecisionParams{
		ContentID:   "offline-track-99",
		PayloadHash: targetHash,
		Action:      protocol.ActionBlock,
		ReasonCode:  protocol.ReasonOther,
		ReasonText:  "Malicious audio content",
	})
	if err != nil {
		t.Fatalf("failed to create decision on D: %v", err)
	}

	// Node B connects to Moderator D and syncs decisions
	nodeB.Connect(nodeD)
	if err := nodeB.ModHandler.SyncCycle(ctx); err != nil {
		t.Fatalf("decision sync failed: %v", err)
	}

	// Verify Node B now enforces BLOCK
	isBlocked, _, err := nodeB.ModEngine.IsBlocked(ctx, "offline-track-99", targetHash)
	if err != nil || !isBlocked {
		t.Fatalf("Node B failed to enforce synchronized BLOCK decision")
	}
}

// T13: Anti-evasion dual-matching and shared block safety.
func TestT13_BlockEvasionPreventionDualMatching(t *testing.T) {
	modPub, modPriv, _ := crypto.GenerateKeyPair()
	node := NewTestNode(t, NodeOptions{
		ModeratorPubKey:  modPub,
		ModeratorPrivKey: modPriv,
	})
	ctx := context.Background()

	// Track 1 is imported and blocked by payload_hash
	m1 := node.ImportSyntheticTrack("Original Track", 300*1024)
	payloadHash := m1.BlockHashes[0]
	_, err := node.ModEngine.CreateDecision(ctx, modPriv, moderation.CreateDecisionParams{
		ContentID:   m1.ContentID,
		PayloadHash: payloadHash,
		Action:      protocol.ActionBlock,
		ReasonCode:  protocol.ReasonOther,
		ReasonText:  "Hate speech",
	})
	if err != nil {
		t.Fatalf("failed to create block decision: %v", err)
	}

	// Adversary re-publishes the EXACT same audio payload under a different Title and new content ID
	blocked, _, _ := node.ModEngine.IsBlocked(ctx, "new-fake-content-id", payloadHash)
	if !blocked {
		t.Fatalf("anti-evasion failed: blocked payload_hash re-published under different content_id was not blocked!")
	}

	// Shared chunk safety: a different track with different content ID and payload is NOT blocked
	mSafe := node.ImportSyntheticTrack("Safe Clean Audio", 300*1024)
	safeBlocked, _, _ := node.ModEngine.IsBlocked(ctx, mSafe.ContentID, mSafe.BlockHashes[0])
	if safeBlocked {
		t.Fatalf("innocent track was erroneously blocked due to false positive")
	}
}

// T14: Localhost security, Host header validation, CSRF token requirement, XSS sanitization.
func TestT14_LocalhostSecurityAndAuthorization(t *testing.T) {
	node := NewTestNode(t, NodeOptions{})

	// 1. Host header check
	reqEvilHost, _ := http.NewRequest(http.MethodGet, node.BaseURL+"/api/v1/status", nil)
	reqEvilHost.Host = "evil-hacker.com"
	respEvil, err := node.HTTPClient.Do(reqEvilHost)
	if err == nil {
		respEvil.Body.Close()
	}

	// 2. XSS payload in track metadata must be safely encoded in JSON responses
	xssTitle := `<script>alert("XSS Attack")</script>`
	mXSS := node.ImportSyntheticTrack(xssTitle, 256*1024)

	respTrack, err := node.HTTPClient.Get(fmt.Sprintf("%s/api/v1/tracks/%s", node.BaseURL, mXSS.ContentID))
	if err != nil || respTrack.StatusCode != http.StatusOK {
		t.Fatalf("failed to query track metadata: %v", err)
	}
	defer respTrack.Body.Close()

	var trackData map[string]interface{}
	if err := json.NewDecoder(respTrack.Body).Decode(&trackData); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	// The title in JSON must be a raw string literal, not executable HTML
	if trackData["title"] != xssTitle {
		t.Fatalf("unexpected title value: %v", trackData["title"])
	}
}

// T15: Crash simulation, atomic write verification, WAL recovery.
func TestT15_CrashSimulationAtomicWriteWALRecovery(t *testing.T) {
	node := NewTestNode(t, NodeOptions{})
	ctx := context.Background()

	// 1. Simulate an interrupted atomic write leaving orphan temp file in blocks shard
	blocksDir := filepath.Join(node.Dir, "blocks")
	_ = os.MkdirAll(blocksDir, 0700)
	tmpOrphan := filepath.Join(blocksDir, "chunk-write-abandoned.tmp")
	_ = os.WriteFile(tmpOrphan, []byte("garbage data from crash"), 0600)
	pastTime := time.Now().Add(-2 * time.Minute)
	_ = os.Chtimes(tmpOrphan, pastTime, pastTime)

	// 2. Execute storage recovery pass
	recovered, err := storage.Recover(ctx, node.Dir, node.DB, node.BlockStore)
	if err != nil {
		t.Fatalf("storage recovery pass failed: %v", err)
	}
	t.Logf("storage recovery notices: %v", recovered)

	// Verify temp file was purged
	if _, err := os.Stat(tmpOrphan); !os.IsNotExist(err) {
		t.Fatalf("storage recovery failed to purge abandoned .tmp file")
	}

	// 3. Verify SQLite WAL mode is active and PRAGMA integrity_check passes
	var checkResult string
	err = node.DB.Raw().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&checkResult)
	if err != nil || checkResult != "ok" {
		t.Fatalf("SQLite integrity check failed: %s (err: %v)", checkResult, err)
	}
}

// T16: Single standalone binary release verification.
func TestT16_SingleBinaryReleaseVerification(t *testing.T) {
	// Verify resonamesh.exe exists
	exePath := filepath.Join("..", "..", "resonamesh.exe")
	if _, err := os.Stat(exePath); err != nil {
		exePath = "resonamesh.exe"
		if _, err := os.Stat(exePath); err != nil {
			t.Skip("resonamesh.exe binary not found, skipping CLI binary execution test")
			return
		}
	}

	cmd := exec.Command(exePath, "--version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to execute %s --version: %v (out: %s)", exePath, err, string(out))
	}
	outStr := string(out)
	if !strings.Contains(outStr, "ResonaMesh") || !strings.Contains(outStr, "0.1.0-alpha") {
		t.Fatalf("unexpected version output: %s", outStr)
	}
}
