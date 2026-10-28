package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

const (
	// DefaultBootstrapHTTPTimeout is the maximum duration allocated to resolve an HTTP/HTTPS seed endpoint.
	DefaultBootstrapHTTPTimeout = 6 * time.Second

	// MaxBootstrapResponseBytes limits HTTP seed body size to 512 KiB.
	MaxBootstrapResponseBytes = 512 * 1024

	// DefaultBootstrapInterval is the periodic interval for re-resolving seeds and reconnecting.
	DefaultBootstrapInterval = 5 * time.Minute
)

// SeedResolver parses multiaddress strings and remote JSON endpoints to peer.AddrInfo descriptors.
type SeedResolver struct {
	httpClient *http.Client
}

// NewSeedResolver creates a seed resolver with a bounded HTTP client.
func NewSeedResolver(client *http.Client) *SeedResolver {
	if client == nil {
		client = &http.Client{
			Timeout: DefaultBootstrapHTTPTimeout,
		}
	}
	return &SeedResolver{httpClient: client}
}

// ResolveSeeds resolves a heterogeneous list of seed endpoints (URLs and multiaddresses).
func (r *SeedResolver) ResolveSeeds(ctx context.Context, seeds []string) ([]peer.AddrInfo, error) {
	peerMap := make(map[peer.ID]*peer.AddrInfo)

	for _, rawSeed := range seeds {
		seed := strings.TrimSpace(rawSeed)
		if seed == "" {
			continue
		}

		if strings.HasPrefix(seed, "http://") || strings.HasPrefix(seed, "https://") {
			remoteAddrs, err := r.fetchFromURL(ctx, seed)
			if err != nil {
				slog.Warn("Failed to resolve bootstrap seed URL", "url", seed, "error", err)
				continue
			}
			for _, pi := range remoteAddrs {
				mergeAddrInfo(peerMap, pi)
			}
		} else {
			pi, err := parseMultiaddrString(seed)
			if err != nil {
				slog.Warn("Ignoring malformed bootstrap multiaddress", "addr", seed, "error", err)
				continue
			}
			mergeAddrInfo(peerMap, *pi)
		}
	}

	result := make([]peer.AddrInfo, 0, len(peerMap))
	for _, pi := range peerMap {
		result = append(result, *pi)
	}

	return result, nil
}

// fetchFromURL retrieves and decodes a JSON list of multiaddress strings from a remote endpoint.
func (r *SeedResolver) fetchFromURL(ctx context.Context, endpoint string) ([]peer.AddrInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed to construct request for %s: %w", endpoint, err)
	}
	req.Header.Set("User-Agent", "ResonaMesh-Bootstrap/0.1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: HTTP request failed for %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bootstrap: HTTP status %d received from %s", resp.StatusCode, endpoint)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxBootstrapResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("bootstrap: failed reading response from %s: %w", endpoint, err)
	}

	var rawAddrs []string
	if err := json.Unmarshal(body, &rawAddrs); err != nil {
		return nil, fmt.Errorf("bootstrap: failed parsing JSON array from %s: %w", endpoint, err)
	}

	var resolved []peer.AddrInfo
	for _, addrStr := range rawAddrs {
		addrStr = strings.TrimSpace(addrStr)
		if addrStr == "" {
			continue
		}
		pi, err := parseMultiaddrString(addrStr)
		if err != nil {
			slog.Warn("Ignoring invalid multiaddress from bootstrap URL", "url", endpoint, "addr", addrStr, "error", err)
			continue
		}
		resolved = append(resolved, *pi)
	}

	return resolved, nil
}

// parseMultiaddrString converts a multiaddr string (with /p2p/ ID component) to peer.AddrInfo.
func parseMultiaddrString(addrStr string) (*peer.AddrInfo, error) {
	pi, err := peer.AddrInfoFromString(addrStr)
	if err == nil {
		return pi, nil
	}

	ma, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return nil, err
	}

	pi, err = peer.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return nil, fmt.Errorf("multiaddress missing peer ID component: %w", err)
	}
	return pi, nil
}

func mergeAddrInfo(m map[peer.ID]*peer.AddrInfo, pi peer.AddrInfo) {
	existing, ok := m[pi.ID]
	if !ok {
		addrs := make([]multiaddr.Multiaddr, len(pi.Addrs))
		copy(addrs, pi.Addrs)
		m[pi.ID] = &peer.AddrInfo{
			ID:    pi.ID,
			Addrs: addrs,
		}
		return
	}

	seen := make(map[string]struct{}, len(existing.Addrs))
	for _, a := range existing.Addrs {
		seen[a.String()] = struct{}{}
	}

	for _, a := range pi.Addrs {
		if _, exists := seen[a.String()]; !exists {
			existing.Addrs = append(existing.Addrs, a)
			seen[a.String()] = struct{}{}
		}
	}
}

// BootstrapSwarm resolves seeds and establishes connections to reachable remote peers.
func BootstrapSwarm(ctx context.Context, h *Host, tracker *PeerTracker, seeds []string, onConnected func(peer.ID)) (int, error) {
	if len(seeds) == 0 {
		return 0, nil
	}

	resolver := NewSeedResolver(nil)
	peerInfos, err := resolver.ResolveSeeds(ctx, seeds)
	if err != nil {
		return 0, err
	}

	if len(peerInfos) == 0 {
		return 0, nil
	}

	var (
		connectedCount int
		wg             sync.WaitGroup
		mu             sync.Mutex
	)

	for _, pi := range peerInfos {
		if pi.ID == h.ID() {
			continue
		}

		if tracker != nil {
			tracker.RecordPeer(pi, false)
		}

		wg.Add(1)
		go func(info peer.AddrInfo) {
			defer wg.Done()

			dialCtx, cancel := context.WithTimeout(ctx, ConnectTimeout)
			defer cancel()

			if err := h.Connect(dialCtx, info); err != nil {
				slog.Debug("Bootstrap dial failed", "peer_id", info.ID.String(), "error", err)
				return
			}

			if tracker != nil {
				tracker.SetConnected(info.ID, true)
			}

			mu.Lock()
			connectedCount++
			mu.Unlock()

			slog.Info("Successfully established connection to bootstrap peer", "peer_id", info.ID.String())

			if onConnected != nil {
				onConnected(info.ID)
			}
		}(pi)
	}

	wg.Wait()
	return connectedCount, nil
}

// StartBootstrapLoop runs periodic bootstrap discovery in the background until the context cancels.
func StartBootstrapLoop(ctx context.Context, h *Host, tracker *PeerTracker, seeds []string, interval time.Duration, onConnected func(peer.ID)) {
	if len(seeds) == 0 {
		return
	}
	if interval <= 0 {
		interval = DefaultBootstrapInterval
	}

	go func() {
		// Initial resolution pass
		if count, err := BootstrapSwarm(ctx, h, tracker, seeds, onConnected); err != nil {
			slog.Warn("Initial bootstrap swarm pass encountered an issue", "error", err)
		} else if count > 0 {
			slog.Info("Initial bootstrap pass connected peers", "count", count)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if count, err := BootstrapSwarm(ctx, h, tracker, seeds, onConnected); err != nil {
					slog.Debug("Periodic bootstrap refresh encountered an issue", "error", err)
				} else if count > 0 {
					slog.Info("Periodic bootstrap refresh connected new peers", "count", count)
				}
			}
		}
	}()
}
