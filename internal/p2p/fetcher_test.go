package p2p

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

type mockCatalogRecorder struct {
	storedCalls int
	lastID      string
	lastIndex   int
	lastSize    int
}

func (m *mockCatalogRecorder) MarkBlockStored(_ context.Context, contentID string, blockIndex int, sizeBytes int) error {
	m.storedCalls++
	m.lastID = contentID
	m.lastIndex = blockIndex
	m.lastSize = sizeBytes
	return nil
}

func TestFetcherOperations(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(filepath.Join(tmpDir, "nodeA"))
	if err != nil {
		t.Fatalf("failed to init block store: %v", err)
	}

	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key A: %v", err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key B: %v", err)
	}

	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host A: %v", err)
	}
	defer hostA.Close()

	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host B: %v", err)
	}
	defer hostB.Close()

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(hostA, bs, mockCat)

	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte((i * 31) % 256)
	}
	validHash := crypto.HashChunkHex(chunk)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Fetch when no peers connected
	_, err = fetcher.FetchBlock(ctx, "track-1", 0, validHash)
	if !errors.Is(err, ErrNoSwarmPeersAvailable) {
		t.Fatalf("expected ErrNoSwarmPeersAvailable, got %v", err)
	}

	// 2. Connect to Host B (which has the block)
	providerB := &memoryProvider{
		blocks: map[string][]byte{
			validHash: chunk,
		},
	}
	NewProtocolHandler(hostB, providerB)

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect A to B: %v", err)
	}

	// 3. Successful remote fetch and disk commit
	fetchedData, err := fetcher.FetchBlock(ctx, "track-1", 0, validHash)
	if err != nil {
		t.Fatalf("expected successful block fetch, got %v", err)
	}
	if len(fetchedData) != len(chunk) {
		t.Fatalf("expected %d bytes, got %d", len(chunk), len(fetchedData))
	}

	// Verify catalog recorded
	if mockCat.storedCalls != 1 || mockCat.lastIndex != 0 || mockCat.lastSize != len(chunk) {
		t.Fatalf("expected catalog recorded once, got %+v", mockCat)
	}

	// Verify local blockstore has it now
	if !bs.Has(validHash) {
		t.Fatalf("expected block to be committed to local blockstore")
	}

	// 4. Local cache hit test (fetcher should read directly from bs without network)
	cachedData, err := fetcher.FetchBlock(ctx, "track-1", 0, validHash)
	if err != nil {
		t.Fatalf("expected cache hit, got %v", err)
	}
	if len(cachedData) != len(chunk) {
		t.Fatalf("mismatched cached data size")
	}
	// mockCat should not have incremented for local cache hit
	if mockCat.storedCalls != 1 {
		t.Fatalf("expected mockCat not incremented on cache hit")
	}

	// 5. Query missing block on connected peer
	missingHash := "0000000000000000000000000000000000000000000000000000000000000000"
	_, err = fetcher.FetchBlock(ctx, "track-1", 1, missingHash)
	if !errors.Is(err, ErrBlockFetchFailed) {
		t.Fatalf("expected ErrBlockFetchFailed, got %v", err)
	}
}

func TestFetcherQuotaGracefulDegradation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bsB, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeB"))
	dbB, _ := storage.Open(filepath.Join(tmpDir, "nodeB", "catalog.db"))
	defer dbB.Close()

	// Node B has a tiny quota of 10 bytes (cannot fit any 256 KiB chunk)
	qmB := storage.NewQuotaManager(10, filepath.Join(tmpDir, "nodeB"), dbB, bsB, nil)

	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

	hostA, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	hostB, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	chunk := []byte("audio-chunk-sample-data")
	validHash := crypto.HashChunkHex(chunk)
	providerA := &memoryProvider{
		blocks: map[string][]byte{
			validHash: chunk,
		},
	}
	_ = NewProtocolHandler(hostA, providerA)

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(hostB, bsB, mockCat)
	fetcher.SetQuotaManager(qmB)

	// Connect hosts
	if err := hostB.Connect(ctx, hostA.AddrInfo()); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Fetch block. Even though quota is exceeded and chunk cannot be cached on disk,
	// it must still return data to the caller for transient streaming without failing the audio session.
	data, err := fetcher.FetchBlock(ctx, "track-1", 0, validHash)
	if err != nil {
		t.Fatalf("expected successful streaming return even with full quota, got: %v", err)
	}
	if len(data) != len(chunk) {
		t.Fatalf("mismatched data length")
	}

	// Must NOT be stored in local blockstore or recorded in catalog
	if bsB.Has(validHash) {
		t.Fatal("chunk must NOT be committed to disk when quota is exceeded")
	}
	if mockCat.storedCalls != 0 {
		t.Fatalf("catalog must not be marked stored when quota is exceeded")
	}
}


func TestFetcherReputationTracking(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bsA, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeA"))
	bsB, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeB"))

	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

	hostA, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	hostB, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	chunk := []byte("high-quality-audio-sample")
	validHash := crypto.HashChunkHex(chunk)
	_ = bsB.Put(validHash, chunk)

	_ = NewProtocolHandler(hostB, &memoryProvider{
		blocks: map[string][]byte{validHash: chunk},
	})

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(hostA, bsA, mockCat)

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect hostA to hostB: %v", err)
	}

	// Initial score should be 100
	if fetcher.ReputationTracker().Score(hostB.ID()) != BaseReputationScore {
		t.Fatalf("expected initial score %f", BaseReputationScore)
	}

	// Fetch block successfully
	data, err := fetcher.FetchBlock(ctx, "track-rep-1", 0, validHash)
	if err != nil {
		t.Fatalf("failed to fetch block: %v", err)
	}
	if len(data) != len(chunk) {
		t.Fatalf("expected %d bytes, got %d", len(chunk), len(data))
	}

	// Verify reputation updated
	stats, ok := fetcher.ReputationTracker().GetStats(hostB.ID())
	if !ok {
		t.Fatalf("expected reputation stats to exist for hostB")
	}
	if stats.BlocksVerified != 1 {
		t.Fatalf("expected 1 verified block, got %d", stats.BlocksVerified)
	}
	if stats.AvgLatencyMs <= 0 {
		t.Fatalf("expected positive latency, got %f", stats.AvgLatencyMs)
	}
}

func TestFetcherQuarantineOnCorruptedBlock(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bsA, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeA"))

	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

	hostA, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	hostB, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	// Host B has corrupted data for the requested hash
	expectedChunk := []byte("expected-clean-audio")
	expectedHash := crypto.HashChunkHex(expectedChunk)
	corruptedChunk := []byte("tampered-audio-payload")

	_ = NewProtocolHandler(hostB, &memoryProvider{
		blocks: map[string][]byte{expectedHash: corruptedChunk},
	})

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(hostA, bsA, mockCat)

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect hostA to hostB: %v", err)
	}

	// Attempt fetch: should fail due to hash mismatch
	_, err := fetcher.FetchBlock(ctx, "track-corrupt", 0, expectedHash)
	if err == nil {
		t.Fatal("expected fetch to fail on hash mismatch")
	}

	// Host B must now be quarantined
	if !fetcher.ReputationTracker().IsQuarantinedNow(hostB.ID()) {
		t.Fatal("expected Host B to be quarantined after delivering corrupted block")
	}

	stats, _ := fetcher.ReputationTracker().GetStats(hostB.ID())
	if stats.HashMismatchCount != 1 {
		t.Fatalf("expected 1 hash mismatch, got %d", stats.HashMismatchCount)
	}

	// Subsequent candidate search should filter out Host B
	prioritized := fetcher.prioritizedSwarmPeers()
	for _, p := range prioritized {
		if p == hostB.ID() {
			t.Fatalf("quarantined Host B must be excluded from candidate list")
		}
	}
}

func TestFetcherPrioritizesHighestReputationPeer(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	bsA, _ := storage.NewBlockStore(filepath.Join(tmpDir, "nodeA"))

	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()
	_, privC, _ := crypto.GenerateKeyPair()

	hostA, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	defer hostA.Close()

	hostB, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	defer hostB.Close()

	hostC, _ := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privC,
		DisableRelay: true,
	})
	defer hostC.Close()

	_ = hostA.Connect(ctx, hostB.AddrInfo())
	_ = hostA.Connect(ctx, hostC.AddrInfo())

	mockCat := &mockCatalogRecorder{}
	fetcher := NewFetcher(hostA, bsA, mockCat)

	// Host C has verified 10 blocks (score ~ 120)
	for i := 0; i < 10; i++ {
		fetcher.ReputationTracker().RecordSuccess(hostC.ID(), 20*time.Millisecond, 256)
	}

	// Host B has 1 stream error (score ~ 85)
	fetcher.ReputationTracker().RecordFailure(hostB.ID(), FailureStreamError)

	prioritized := fetcher.prioritizedSwarmPeers()
	if len(prioritized) < 2 {
		t.Fatalf("expected at least 2 connected peers, got %d", len(prioritized))
	}

	// Host C must be prioritized ahead of Host B
	if prioritized[0] != hostC.ID() {
		t.Fatalf("expected highest reputation peer hostC (%s) to be first, got %s", hostC.ID(), prioritized[0])
	}
}
