package p2p

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
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

// BlockChecker evaluates whether audio content is blocked by moderation policy.
type BlockChecker interface {
	IsBlocked(ctx context.Context, contentID string, payloadHash string) (bool, string, error)
}

// Fetcher coordinates retrieval of missing audio chunks from the P2P swarm.
type Fetcher struct {
	host         *Host
	blockStore   *storage.BlockStore
	catalog      CatalogRecorder
	quotaManager *storage.QuotaManager
	blockChecker BlockChecker
	reputation   *PeerReputationTracker
}

// NewFetcher creates a new swarm block fetcher with a default reputation tracker.
func NewFetcher(h *Host, bs *storage.BlockStore, cat CatalogRecorder) *Fetcher {
	return &Fetcher{
		host:       h,
		blockStore: bs,
		catalog:    cat,
		reputation: NewPeerReputationTracker(),
	}
}

// SetQuotaManager binds the storage quota manager to validate disk allowances before caching blocks.
func (f *Fetcher) SetQuotaManager(qm *storage.QuotaManager) {
	f.quotaManager = qm
}

// SetBlockChecker binds a moderation policy checker to guard against fetching blocked content.
func (f *Fetcher) SetBlockChecker(bc BlockChecker) {
	f.blockChecker = bc
}

// SetReputationTracker binds a custom peer reputation tracker.
func (f *Fetcher) SetReputationTracker(rt *PeerReputationTracker) {
	f.reputation = rt
}

// ReputationTracker returns the peer reputation tracker bound to this fetcher.
func (f *Fetcher) ReputationTracker() *PeerReputationTracker {
	return f.reputation
}

// FetchBlock retrieves an audio block by its hash. It first checks the local block store;
// if absent, it queries connected swarm peers, verifies SHA-256 integrity, stores the block locally,
// updates the catalog availability record, and returns the raw block bytes.
func (f *Fetcher) FetchBlock(ctx context.Context, contentID string, blockIndex int, blockHash string) ([]byte, error) {
	// Guard: check moderation policy
	if f.blockChecker != nil {
		blocked, reason, err := f.blockChecker.IsBlocked(ctx, contentID, "")
		if err == nil && blocked {
			return nil, fmt.Errorf("p2p: content is blocked by moderation policy: %s", reason)
		}
	}

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

	// 2. Discover available swarm peers, prioritizing direct P2P swarm nodes ahead of WebSocket bootstrap relays
	peers := f.prioritizedSwarmPeers()
	if len(peers) == 0 {
		slog.Warn("No connected swarm peers available to fetch block", "hash", blockHash)
		return nil, ErrNoSwarmPeersAvailable
	}
	slog.Info("Querying swarm peers for block", "hash", blockHash, "index", blockIndex, "peers_count", len(peers))

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

		startTime := time.Now()
		peerCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		data, err := RequestBlock(peerCtx, f.host, peerID, req)
		elapsed := time.Since(startTime)
		cancel()

		if err != nil {
			if f.reputation != nil {
				if errors.Is(err, ErrBlockHashMismatch) {
					f.reputation.RecordFailure(peerID, FailureHashMismatch)
				} else if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "deadline") || strings.Contains(err.Error(), "timeout") {
					f.reputation.RecordFailure(peerID, FailureTimeout)
				} else {
					f.reputation.RecordFailure(peerID, FailureStreamError)
				}
			}
			slog.Warn("Swarm peer failed to supply block",
				"peer_id", peerID.String(),
				"hash", blockHash,
				"err", err,
			)
			continue
		}

		if f.reputation != nil {
			f.reputation.RecordSuccess(peerID, elapsed, len(data))
		}

		// 4. Reserve disk space and persist to local blockstore
		var resToken storage.ReservationToken
		shouldStore := true

		if f.quotaManager != nil {
			tok, err := f.quotaManager.Reserve(ctx, int64(len(data)))
			if err != nil {
				slog.Warn("Storage quota exceeded, streaming chunk without local caching",
					"hash", blockHash,
					"err", err,
				)
				shouldStore = false
			} else {
				resToken = tok
			}
		}

		if shouldStore && f.blockStore != nil {
			if err := f.blockStore.Put(blockHash, data); err != nil {
				slog.Warn("Failed to persist downloaded swarm block to disk",
					"hash", blockHash,
					"err", err,
				)
				if f.quotaManager != nil && resToken.ID != "" {
					f.quotaManager.Release(resToken)
					resToken = storage.ReservationToken{}
				}
				shouldStore = false
			}
		}

		// 5. Update catalog availability
		if shouldStore && f.catalog != nil && contentID != "" {
			if err := f.catalog.MarkBlockStored(ctx, contentID, blockIndex, len(data)); err != nil {
				slog.Warn("Failed to update catalog for stored block",
					"content_id", contentID,
					"index", blockIndex,
					"err", err,
				)
			}
		}

		if shouldStore && f.quotaManager != nil && resToken.ID != "" {
			f.quotaManager.Commit(resToken, int64(len(data)))
		}

		connType := "relayed"
		if f.host.HasDirectConnection(peerID) {
			connType = "direct"
		}

		slog.Info("Successfully fetched block from swarm peer",
			"peer_id", peerID.String(),
			"hash", blockHash,
			"bytes", len(data),
			"connection", connType,
		)
		return data, nil
	}

	slog.Warn("Failed to fetch block from any connected peer", "hash", blockHash, "peers_tried", len(peers))
	return nil, ErrBlockFetchFailed
}

// prioritizedSwarmPeers orders connected peers so that direct non-WebSocket P2P peers are queried first,
// followed by circuit-relayed peers, and WebSocket-only bootstrap relays last.
// Quarantined peers are excluded, and candidates within each connection tier are sorted by reputation score.
func (f *Fetcher) prioritizedSwarmPeers() []peer.ID {
	if f.host == nil {
		return nil
	}

	all := f.host.ConnectedPeers()
	if len(all) == 0 {
		return nil
	}

	// 1. Filter out quarantined peers
	now := time.Now()
	var candidates []peer.ID
	for _, pid := range all {
		if f.reputation != nil && f.reputation.IsQuarantined(pid, now) {
			slog.Debug("Filtering out quarantined swarm peer from block fetch candidates", "peer_id", pid.String())
			continue
		}
		candidates = append(candidates, pid)
	}

	if len(candidates) <= 1 {
		return candidates
	}

	var directP2P []peer.ID
	var relayedP2P []peer.ID
	var wsRelays []peer.ID

	for _, pid := range candidates {
		isDirect := false
		isWSOnly := true
		conns := f.host.Network().ConnsToPeer(pid)
		for _, c := range conns {
			addrStr := c.RemoteMultiaddr().String()
			if !c.Stat().Limited && !strings.Contains(addrStr, "p2p-circuit") {
				isDirect = true
			}
			if !strings.Contains(addrStr, "/ws") && !strings.Contains(addrStr, "/wss") {
				isWSOnly = false
			}
		}

		switch {
		case isDirect && !isWSOnly:
			directP2P = append(directP2P, pid)
		case !isWSOnly:
			relayedP2P = append(relayedP2P, pid)
		default:
			wsRelays = append(wsRelays, pid)
		}
	}

	// Sort peers within each tier descending by reputation score
	sortPeersByReputation := func(peers []peer.ID) {
		if f.reputation == nil || len(peers) <= 1 {
			return
		}
		sort.SliceStable(peers, func(i, j int) bool {
			return f.reputation.Score(peers[i]) > f.reputation.Score(peers[j])
		})
	}

	sortPeersByReputation(directP2P)
	sortPeersByReputation(relayedP2P)
	sortPeersByReputation(wsRelays)

	ordered := make([]peer.ID, 0, len(candidates))
	ordered = append(ordered, directP2P...)
	ordered = append(ordered, relayedP2P...)
	ordered = append(ordered, wsRelays...)
	return ordered
}
