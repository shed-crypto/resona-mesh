package storage_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestBlockStorePutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	// 256 KiB block
	chunk := make([]byte, protocol.BlockSize)
	for i := range chunk {
		chunk[i] = byte((i * 7) % 256)
	}
	hash := crypto.HashChunkHex(chunk)

	// Verify initially does not have
	if bs.Has(hash) {
		t.Fatal("expected store not to have block before Put")
	}

	// Put chunk
	if err := bs.Put(hash, chunk); err != nil {
		t.Fatalf("failed to put valid chunk: %v", err)
	}

	if !bs.Has(hash) {
		t.Fatal("expected store to have block after Put")
	}

	// Get chunk and verify bytes
	retrieved, err := bs.Get(hash)
	if err != nil {
		t.Fatalf("failed to get chunk: %v", err)
	}

	if !bytes.Equal(chunk, retrieved) {
		t.Fatal("retrieved chunk payload does not match original bytes")
	}
}

func TestBlockStoreHashMismatchRejection(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	chunk := []byte("audio-chunk-data")
	wrongHash := strings.Repeat("a", 64)

	err = bs.Put(wrongHash, chunk)
	if err != storage.ErrHashMismatch {
		t.Fatalf("expected ErrHashMismatch, got: %v", err)
	}

	if bs.Has(wrongHash) {
		t.Fatal("rejected chunk must not exist on disk")
	}
}

func TestBlockStoreCorruptedDiskRead(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	chunk := []byte("critical-audio-block-payload-12345")
	hash := crypto.HashChunkHex(chunk)

	if err := bs.Put(hash, chunk); err != nil {
		t.Fatalf("failed to put chunk: %v", err)
	}

	// Locate file on disk and tamper with 1 byte
	prefix1 := hash[0:2]
	prefix2 := hash[2:4]
	filePath := filepath.Join(tmpDir, "blocks", prefix1, prefix2, hash)

	tamperedData := append([]byte(nil), chunk...)
	tamperedData[0] ^= 0xFF
	if err := os.WriteFile(filePath, tamperedData, 0600); err != nil {
		t.Fatalf("failed to write tampered data: %v", err)
	}

	// Reading corrupted file must trigger ErrCorruptedBlock
	_, err = bs.Get(hash)
	if err != storage.ErrCorruptedBlock {
		t.Fatalf("expected ErrCorruptedBlock when reading tampered disk block, got: %v", err)
	}
}

func TestBlockStoreShardingDirectoryStructure(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	chunk := []byte("test-sharding-chunk")
	hash := crypto.HashChunkHex(chunk)

	if err := bs.Put(hash, chunk); err != nil {
		t.Fatalf("failed to put chunk: %v", err)
	}

	expectedPrefix1 := hash[0:2]
	expectedPrefix2 := hash[2:4]
	expectedPath := filepath.Join(tmpDir, "blocks", expectedPrefix1, expectedPrefix2, hash)

	fi, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("expected file to exist at 2-level sharded path %s: %v", expectedPath, err)
	}
	if fi.Size() != int64(len(chunk)) {
		t.Fatalf("expected size %d, got %d", len(chunk), fi.Size())
	}
}

func TestBlockStoreDelete(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	chunk := []byte("to-be-deleted-chunk")
	hash := crypto.HashChunkHex(chunk)

	if err := bs.Put(hash, chunk); err != nil {
		t.Fatalf("failed to put chunk: %v", err)
	}

	if err := bs.Delete(hash); err != nil {
		t.Fatalf("failed to delete chunk: %v", err)
	}

	if bs.Has(hash) {
		t.Fatal("deleted chunk should not exist in store")
	}

	_, err = bs.Get(hash)
	if err != storage.ErrBlockNotFound {
		t.Fatalf("expected ErrBlockNotFound after deletion, got: %v", err)
	}
}

func TestBlockStoreExceedsLimit(t *testing.T) {
	tmpDir := t.TempDir()
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create block store: %v", err)
	}

	// 256 KiB + 1 byte
	oversized := make([]byte, protocol.BlockSize+1)
	hash := crypto.HashChunkHex(oversized)

	err = bs.Put(hash, oversized)
	if err != storage.ErrBlockExceedsLimit {
		t.Fatalf("expected ErrBlockExceedsLimit for chunk > 256 KiB, got: %v", err)
	}
}
