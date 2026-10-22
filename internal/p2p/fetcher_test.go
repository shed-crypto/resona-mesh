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
