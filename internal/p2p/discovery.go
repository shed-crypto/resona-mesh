package p2p

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

const (
	// DefaultMDNSServiceTag identifies ResonaMesh nodes in local network mDNS discovery.
	DefaultMDNSServiceTag = "resonamesh-swarm"

	// ConnectTimeout is the maximum duration allocated to establish connection to a discovered peer.
	ConnectTimeout = 5 * time.Second
)

// PeerRecord holds runtime discovery and connectivity state for a swarm peer.
type PeerRecord struct {
	ID        peer.ID       `json:"peer_id"`
	Addrs     []string      `json:"addrs"`
	FirstSeen time.Time     `json:"first_seen"`
	LastSeen  time.Time     `json:"last_seen"`
	Connected bool          `json:"connected"`
}

// PeerTracker tracks known and connected swarm peers in a thread-safe registry.
type PeerTracker struct {
	mu    sync.RWMutex
	peers map[peer.ID]*PeerRecord
}

// NewPeerTracker initializes an empty peer registry.
func NewPeerTracker() *PeerTracker {
	return &PeerTracker{
		peers: make(map[peer.ID]*PeerRecord),
	}
}

// RecordPeer updates or inserts peer record on discovery.
func (pt *PeerTracker) RecordPeer(pi peer.AddrInfo, connected bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	addrStrings := make([]string, len(pi.Addrs))
	for i, a := range pi.Addrs {
		addrStrings[i] = a.String()
	}

	rec, exists := pt.peers[pi.ID]
	if !exists {
		pt.peers[pi.ID] = &PeerRecord{
			ID:        pi.ID,
			Addrs:     addrStrings,
			FirstSeen: now,
			LastSeen:  now,
			Connected: connected,
		}
		return
	}

	rec.LastSeen = now
	rec.Connected = connected
	if len(addrStrings) > 0 {
		rec.Addrs = addrStrings
	}
}

// SetConnected updates the connectivity flag for a specific peer.
func (pt *PeerTracker) SetConnected(id peer.ID, connected bool) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if rec, exists := pt.peers[id]; exists {
		rec.Connected = connected
		rec.LastSeen = time.Now()
	}
}

// Peers returns a snapshot of all tracked peer records.
func (pt *PeerTracker) Peers() []PeerRecord {
	pt.mu.RLock()
	defer pt.mu.RUnlock()

	result := make([]PeerRecord, 0, len(pt.peers))
	for _, rec := range pt.peers {
		result = append(result, *rec)
	}
	return result
}

// Count returns the number of currently tracked peers.
func (pt *PeerTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	return len(pt.peers)
}

// DiscoveryService orchestrates mDNS broadcasting and auto-connection in the local network.
type DiscoveryService struct {
	host       *Host
	service    mdns.Service
	tracker    *PeerTracker
	serviceTag string
}

// NewDiscoveryService creates and binds an mDNS service to the libp2p host.
func NewDiscoveryService(h *Host, tracker *PeerTracker, serviceTag string) *DiscoveryService {
	if serviceTag == "" {
		serviceTag = DefaultMDNSServiceTag
	}
	if tracker == nil {
		tracker = NewPeerTracker()
	}

	ds := &DiscoveryService{
		host:       h,
		tracker:    tracker,
		serviceTag: serviceTag,
	}

	ds.service = mdns.NewMdnsService(h.Raw(), serviceTag, ds)
	return ds
}

// Start begins local mDNS discovery and broadcasting.
func (d *DiscoveryService) Start() error {
	slog.Info("Starting mDNS peer discovery service",
		"service_tag", d.serviceTag,
		"peer_id", d.host.ID().String(),
	)
	if err := d.service.Start(); err != nil {
		return fmt.Errorf("p2p: failed to start mdns service: %w", err)
	}
	return nil
}

// HandlePeerFound implements the mdns.Notifee interface, triggered when a peer announces itself.
func (d *DiscoveryService) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == d.host.ID() {
		return // Ignore self announcement
	}

	slog.Debug("mDNS discovered remote peer", "peer_id", pi.ID.String(), "addrs", pi.Addrs)

	// Record in tracker
	d.tracker.RecordPeer(pi, false)

	// Attempt connection in background
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), ConnectTimeout)
		defer cancel()

		if err := d.host.Connect(ctx, pi); err != nil {
			slog.Debug("Failed to connect to discovered peer", "peer_id", pi.ID.String(), "err", err)
			return
		}

		d.tracker.SetConnected(pi.ID, true)
		slog.Info("Successfully connected to discovered swarm peer", "peer_id", pi.ID.String())
	}()
}

// Tracker returns the underlying PeerTracker.
func (d *DiscoveryService) Tracker() *PeerTracker {
	return d.tracker
}

// Close terminates the mDNS service.
func (d *DiscoveryService) Close() error {
	return d.service.Close()
}
