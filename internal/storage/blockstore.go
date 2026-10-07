package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

var (
	ErrHashMismatch      = errors.New("blockstore: computed payload hash does not match expected hash")
	ErrCorruptedBlock     = errors.New("blockstore: stored block failed integrity check on disk")
	ErrInvalidHashFormat = errors.New("blockstore: invalid block hash format (expected 64 hex characters)")
	ErrBlockNotFound     = errors.New("blockstore: block not found")
	ErrBlockExceedsLimit = errors.New("blockstore: block size exceeds maximum protocol limit")
)

// BlockStore manages content-addressed 256 KiB audio chunks on disk with atomic writes.
type BlockStore struct {
	rootDir string
}

// NewBlockStore initializes the block store directory.
func NewBlockStore(rootDir string) (*BlockStore, error) {
	blocksDir := filepath.Join(rootDir, "blocks")
	if err := os.MkdirAll(blocksDir, 0700); err != nil {
		return nil, fmt.Errorf("blockstore: failed to initialize blocks root: %w", err)
	}
	return &BlockStore{rootDir: blocksDir}, nil
}

// Put validates and writes a chunk atomically using a temporary file and rename.
func (bs *BlockStore) Put(expectedHash string, data []byte) error {
	normalizedHash := strings.ToLower(strings.TrimSpace(expectedHash))
	if len(normalizedHash) != 64 {
		return ErrInvalidHashFormat
	}

	if len(data) > protocol.BlockSize {
		return ErrBlockExceedsLimit
	}

	// Cryptographic validation before writing to disk
	if !crypto.VerifyChunk(data, normalizedHash) {
		return ErrHashMismatch
	}

	targetPath, err := bs.blockPath(normalizedHash)
	if err != nil {
		return err
	}

	// Idempotent check: if already stored and valid, skip rewrite
	if fi, err := os.Stat(targetPath); err == nil && fi.Size() == int64(len(data)) {
		return nil
	}

	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0700); err != nil {
		return fmt.Errorf("blockstore: failed to create chunk shard directory: %w", err)
	}

	// Write to temporary file in the same directory to ensure atomic os.Rename across filesystem boundaries
	tempFile, err := os.CreateTemp(targetDir, "chunk-write-*")
	if err != nil {
		return fmt.Errorf("blockstore: failed to create temp chunk file: %w", err)
	}
	tempPath := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := tempFile.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("blockstore: failed to write chunk payload: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("blockstore: failed to sync chunk file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		cleanup()
		return fmt.Errorf("blockstore: failed to close temp file: %w", err)
	}

	// Atomic rename to final path
	if err := os.Rename(tempPath, targetPath); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("blockstore: failed to commit chunk rename: %w", err)
	}

	return nil
}

// Get reads a block from disk and validates its integrity before returning.
func (bs *BlockStore) Get(hash string) ([]byte, error) {
	path, err := bs.blockPath(hash)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlockNotFound
		}
		return nil, fmt.Errorf("blockstore: failed to open chunk: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("blockstore: failed to read chunk content: %w", err)
	}

	// Verify integrity on read (silent bit-rot protection)
	if !crypto.VerifyChunk(data, hash) {
		return nil, ErrCorruptedBlock
	}

	return data, nil
}

// Has checks whether a block exists on disk.
func (bs *BlockStore) Has(hash string) bool {
	path, err := bs.blockPath(hash)
	if err != nil {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && fi.Size() > 0
}

// Delete removes a block from the store.
func (bs *BlockStore) Delete(hash string) error {
	path, err := bs.blockPath(hash)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("blockstore: failed to delete chunk: %w", err)
	}
	return nil
}

// blockPath generates a 2-level sharded directory path: blocks/ab/cd/abcdef...
func (bs *BlockStore) blockPath(hash string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(hash))
	if len(h) != 64 {
		return "", ErrInvalidHashFormat
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return "", ErrInvalidHashFormat
		}
	}
	prefix1 := h[0:2]
	prefix2 := h[2:4]
	return filepath.Join(bs.rootDir, prefix1, prefix2, h), nil
}
