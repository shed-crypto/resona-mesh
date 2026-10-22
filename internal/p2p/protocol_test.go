package p2p

import (
	"context"
	"errors"
	"testing"
	"time"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

type memoryProvider struct {
	blocks map[string][]byte
}

func (m *memoryProvider) GetBlock(_ context.Context, _ string, _ int, blockHash string) ([]byte, error) {
	data, ok := m.blocks[blockHash]
	if !ok {
		return nil, errors.New("not found")
	}
	return data, nil
}

func TestProtocolBlockExchangeEndToEnd(t *testing.T) {
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

	// Prepare synthetic 256 KiB chunk
	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte((i * 17) % 256)
	}
	validHash := crypto.HashChunkHex(chunk)

	provider := &memoryProvider{
		blocks: map[string][]byte{
			validHash: chunk,
		},
	}

	// Register protocol handler on Host B
	NewProtocolHandler(hostB, provider)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect A to B: %v", err)
	}

	// 1. Successful fetch
	req := WireBlockRequest{
		ContentID:  "test-content-1",
		BlockIndex: 0,
		BlockHash:  validHash,
	}

	received, err := RequestBlock(ctx, hostA, hostB.ID(), req)
	if err != nil {
		t.Fatalf("failed to request valid block: %v", err)
	}
	if len(received) != len(chunk) {
		t.Fatalf("expected %d bytes, got %d", len(chunk), len(received))
	}
	if crypto.HashChunkHex(received) != validHash {
		t.Fatalf("received hash mismatch")
	}

	// 2. Fetch missing block
	missingReq := WireBlockRequest{
		ContentID:  "test-content-1",
		BlockIndex: 1,
		BlockHash:  "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
	}

	_, err = RequestBlock(ctx, hostA, hostB.ID(), missingReq)
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected ErrBlockNotFound, got %v", err)
	}

	// 3. Tampered payload verification failure
	corruptedProvider := &memoryProvider{
		blocks: map[string][]byte{
			validHash: append(chunk[:len(chunk)-1], chunk[len(chunk)-1]^0xFF),
		},
	}
	corruptedHost, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create corrupted host: %v", err)
	}
	defer corruptedHost.Close()

	NewProtocolHandler(corruptedHost, corruptedProvider)
	if err := hostA.Connect(ctx, corruptedHost.AddrInfo()); err != nil {
		t.Fatalf("failed to connect A to corruptedHost: %v", err)
	}

	_, err = RequestBlock(ctx, hostA, corruptedHost.ID(), req)
	if !errors.Is(err, ErrBlockHashMismatch) {
		t.Fatalf("expected ErrBlockHashMismatch, got %v", err)
	}

	// 4. Empty block hash validation error
	_, err = RequestBlock(ctx, hostA, hostB.ID(), WireBlockRequest{})
	if !errors.Is(err, ErrEmptyBlockExpected) {
		t.Fatalf("expected ErrEmptyBlockExpected, got %v", err)
	}
}

type mockWireBlockChecker struct {
	blocked bool
	reason  string
}

func (m *mockWireBlockChecker) IsBlocked(_ context.Context, contentID string, _ string) (bool, string, error) {
	return m.blocked, m.reason, nil
}

func TestProtocolBlockModerationRejection(t *testing.T) {
	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

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

	chunk := []byte("audio-chunk-data")
	validHash := crypto.HashChunkHex(chunk)

	provider := &memoryProvider{
		blocks: map[string][]byte{
			validHash: chunk,
		},
	}

	handler := NewProtocolHandler(hostB, provider)
	checker := &mockWireBlockChecker{
		blocked: true,
		reason:  "Infringing audio",
	}
	handler.SetBlockChecker(checker)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to B: %v", err)
	}

	req := WireBlockRequest{
		ContentID:  "blocked-track-id",
		BlockIndex: 0,
		BlockHash:  validHash,
	}

	_, err = RequestBlock(ctx, hostA, hostB.ID(), req)
	if !errors.Is(err, ErrContentBlocked) {
		t.Fatalf("expected ErrContentBlocked for moderated track, got: %v", err)
	}
}

