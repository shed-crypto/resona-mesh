package p2p

import (
	"context"
	"crypto/ed25519"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/catalog"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

type mockStreamerReporter struct {
	streaming bool
}

func (m *mockStreamerReporter) IsStreaming() bool {
	return m.streaming
}

func TestKeeperYieldsWhenStreaming(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	db, _ := storage.Open(filepath.Join(tmpDir, "catalog.db"))
	defer db.Close()
	bs, _ := storage.NewBlockStore(tmpDir)
	cat := catalog.New(db, bs)

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(nil, bs, mockCat)

	streamer := &mockStreamerReporter{streaming: true}

	keeper := NewKeeper(
		KeeperConfig{Enabled: true, Interval: 10 * time.Millisecond, TargetCopies: 2},
		cat,
		fetcher,
		streamer,
		nil,
	)

	// Run cycle when streaming is true
	if err := keeper.RunCycle(ctx); err != nil {
		t.Fatalf("expected nil error on yield, got: %v", err)
	}

	if mockCat.storedCalls != 0 {
		t.Fatalf("expected keeper not to make any fetch calls when streaming is active")
	}
}

func TestKeeperReplicatesMissingBlocks(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	// 1. Setup Node A (Source Seeder)
	bsA, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeA"))
	_, privA, _ := crypto.GenerateKeyPair()
	hostA, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte((i * 17) % 256)
	}
	hash := crypto.HashChunkHex(chunk)
	_ = bsA.Put(hash, chunk)

	providerA := &memoryProvider{
		blocks: map[string][]byte{
			hash: chunk,
		},
	}
	_ = NewProtocolHandler(hostA, providerA)

	// 2. Setup Node B (Replication Target)
	bsB, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeB"))
	dbB, _ := storage.Open(filepath.Join(tmpDir, "nodeB", "catalog.db"))
	defer dbB.Close()
	catB := catalog.New(dbB, bsB)

	_, privB, _ := crypto.GenerateKeyPair()
	hostB, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	fetcherB := NewFetcher(hostB, bsB, catB)
	streamerB := &mockStreamerReporter{streaming: false}

	// Register track on Node B as an unseeded remote track (is_stored = 0)
	manifest := &protocol.Manifest{
		SchemaVersion:   "1.0.0",
		ContentID:       "track_keeper_test",
		Title:           "Ambient Swarm",
		Artist:          "Autonomous Synth",
		DurationMs:      120000,
		BitrateKbps:     192,
		TotalSizeBytes:  int64(len(chunk)),
		BlockSize:       protocol.BlockSize,
		BlockHashes:     []string{hash},
		PublisherKeyHex: crypto.PublicKeyToHex(privA.Public().(ed25519.PublicKey)),
		PublishedAt:     time.Now().Unix(),
		LicenseCode:     protocol.LicenseCC0,
		Signature:       "valid_sig",
	}
	if err := catB.RegisterRemoteManifest(ctx, manifest); err != nil {
		t.Fatalf("failed to register remote manifest: %v", err)
	}

	// Verify initially 0% availability on Node B
	trackBefore, _ := catB.GetTrack(ctx, manifest.ContentID)
	if trackBefore.AvailabilityPct != 0.0 {
		t.Fatalf("expected 0%% availability before keeper replication, got: %.1f%%", trackBefore.AvailabilityPct)
	}

	// Connect Node B to Node A
	if err := hostB.Connect(ctx, hostA.AddrInfo()); err != nil {
		t.Fatalf("failed to connect B to A: %v", err)
	}

	// 3. Run Keeper replication cycle on Node B
	keeperB := NewKeeper(
		KeeperConfig{Enabled: true, Interval: 50 * time.Millisecond, TargetCopies: 2},
		catB,
		fetcherB,
		streamerB,
		nil,
	)

	if err := keeperB.RunCycle(ctx); err != nil {
		t.Fatalf("keeper cycle failed: %v", err)
	}

	// 4. Verify Node B now has the block in blockstore and catalog is updated
	if !bsB.Has(hash) {
		t.Fatal("expected block to be replicated and saved to Node B blockstore")
	}

	trackAfter, _ := catB.GetTrack(ctx, manifest.ContentID)
	if trackAfter.AvailabilityPct != 100.0 {
		t.Fatalf("expected track to have 100%% availability after replication, got: %.1f%%", trackAfter.AvailabilityPct)
	}
}
