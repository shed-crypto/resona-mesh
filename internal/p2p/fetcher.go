package p2p

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"resonamesh/internal/storage"
)

var (
	ErrNoSwarmPeersAvailable = errors.New("p2p: no connected swarm peers available to fetch block")
	ErrBlockFetchFailed      = errors.New("p2p: failed to fetch block from any connected peer")
)

// CatalogRecorder updates catalog block availability states when new chunks are fetched.
type CatalogRecorder interface {
	MarkBlockStored(ctx context.Context, contentID string, blockIndex int, sizeBytes int) error
}

// Fetcher coordinates retrieval of missing audio chunks from the P2P swarm.
type Fetcher struct {
	host       *Host
	blockStore *storage.BlockStore
	catalog    CatalogRecorder
}

// NewFetcher creates a new swarm block fetcher.
func NewFetcher(h *Host, bs *storage.BlockStore, cat CatalogRecorder) *Fetcher {
	return &Fetcher{
		host:       h,
		blockStore: bs,
		catalog:    cat,
	}
}

// FetchBlock retrieves an audio block by its hash. It first checks the local block store;
// if absent, it queries connected swarm peers, verifies SHA-256 integrity, stores the block locally,
// updates the catalog availability record, and returns the raw block bytes.
func (f *Fetcher) FetchBlock(ctx context.Context, contentID string, blockIndex int, blockHash string) ([]byte, error) {
	// 1. Check local blockstore cache hit
	if f.blockStore != nil && f.blockStore.Has(blockHash) {
		data, err := f.blockStore.Get(blockHash)
		if err == nil {
			return data, nil
		}
		slog.Warn("Local block corrupted or inaccessible, falling back to swarm fetch",
			"hash", blockHash,
			"err", err,
		)
	}

	// 2. Discover available swarm peers
	peers := f.host.ConnectedPeers()
	if len(peers) == 0 {
		return nil, ErrNoSwarmPeersAvailable
	}

	req := WireBlockRequest{
		ContentID:  contentID,
		BlockIndex: blockIndex,
		BlockHash:  blockHash,
	}

	// 3. Attempt download from connected peers
	for _, peerID := range peers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		peerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		data, err := RequestBlock(peerCtx, f.host, peerID, req)
		cancel()

		if err != nil {
			slog.Debug("Swarm peer failed to supply block",
				"peer_id", peerID.String(),
				"hash", blockHash,
				"err", err,
			)
			continue
		}

		// 4. Persist to local blockstore
		if f.blockStore != nil {
			if err := f.blockStore.Put(blockHash, data); err != nil {
				slog.Warn("Failed to persist downloaded swarm block to disk",
					"hash", blockHash,
					"err", err,
				)
			}
		}

		// 5. Update catalog availability
		if f.catalog != nil && contentID != "" {
			if err := f.catalog.MarkBlockStored(ctx, contentID, blockIndex, len(data)); err != nil {
				slog.Warn("Failed to update catalog for stored block",
					"content_id", contentID,
					"index", blockIndex,
					"err", err,
				)
			}
		}

		slog.Info("Successfully fetched block from swarm peer",
			"peer_id", peerID.String(),
			"hash", blockHash,
			"bytes", len(data),
		)
		return data, nil
	}

	return nil, ErrBlockFetchFailed
}
