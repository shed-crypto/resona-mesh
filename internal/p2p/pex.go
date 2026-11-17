package p2p

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

const (
	PEXProtocolID       = protocol.ID("/resonamesh/pex/1.0.0")
	PEXTimeout          = 8 * time.Second
	MaxPrimaryRelays    = 16
	MaxBackupRelays     = 32
	MaxNextHopsPerRelay = 4
	MaxReferralSample   = 4
	RelayStaleTTL       = 15 * time.Minute
)

var (
	ErrNoRelayAvailable = errors.New("pex: no suitable circuit relay available")
)

// RelayDescriptor advertises a candidate relay node with observed capacity and 2-hop backup paths.
type RelayDescriptor struct {
	ID           peer.ID             `json:"id"`
	Addrs        []string            `json:"addrs"`
	Mode         string              `json:"mode"`                     // "coordinator", "transit", "seed"
	LoadPct      int                 `json:"load_pct"`                 // 0..100
	NextHopIDs   []peer.ID           `json:"next_hop_ids,omitempty"`   // 2-hop backup relays
	NextHopAddrs map[string][]string `json:"next_hop_addrs,omitempty"` // peerID string -> multiaddrs
	LastSeen     int64               `json:"last_seen"`
	Failures     int                 `json:"-"`
}

type pexRequest struct {
	Type       string           `json:"type"`                  // "get_peers" or "find_relay"
	ExcludeIDs []peer.ID        `json:"exclude_ids,omitempty"` // Visited relays in current search chain
	Exhausted  bool             `json:"exhausted,omitempty"`   // True if client exhausted all downstream referrals
	SelfRelay  *RelayDescriptor `json:"self_relay,omitempty"`  // Announcement if caller is an active relay
}

type pexResponse struct {
	Peers         []peer.AddrInfo   `json:"peers"`
	Relays        []RelayDescriptor `json:"relays,omitempty"`
	AcceptReserve bool              `json:"accept_reserve,omitempty"`
}

// PeerExchange coordinates active swarm peer discovery and smart relay orchestration.
type PeerExchange struct {
	host               *Host
	tracker            *PeerTracker
	disableRelay       bool
	localRelayMode     string
	isSeedNode         bool
	maxReservations    int
	activeReservations atomic.Int32
	minSwarmPeers      int
	maxSwarmPeers      int

	relaysMu      sync.RWMutex
	primaryRelays map[peer.ID]*RelayDescriptor
	backupRelays  map[peer.ID]*RelayDescriptor

	activeReservationMu sync.Mutex
	reservedRelayID     peer.ID
}

// NewPeerExchange creates and registers the PEX wire protocol on the host.
func NewPeerExchange(h *Host, tracker *PeerTracker) *PeerExchange {
	pe := &PeerExchange{
		host:            h,
		tracker:         tracker,
		localRelayMode:  "coordinator",
		maxReservations: 16,
		minSwarmPeers:   3,
		maxSwarmPeers:   32,
		primaryRelays:   make(map[peer.ID]*RelayDescriptor),
		backupRelays:    make(map[peer.ID]*RelayDescriptor),
	}
	h.raw.SetStreamHandler(PEXProtocolID, pe.handleStream)
	return pe
}

// SetDisableRelay controls whether circuit relay addresses are appended in PEX.
func (pe *PeerExchange) SetDisableRelay(disabled bool) {
	pe.disableRelay = disabled
}

// SetRelayConfig sets relay orchestration parameters.
func (pe *PeerExchange) SetRelayConfig(mode string, isSeed bool, maxReservations int, minSwarmPeers int, maxSwarmPeers int) {
	pe.localRelayMode = mode
	pe.isSeedNode = isSeed
	if maxReservations > 0 {
		pe.maxReservations = maxReservations
	}
	if minSwarmPeers > 0 {
		pe.minSwarmPeers = minSwarmPeers
	}
	if maxSwarmPeers > 0 {
		pe.maxSwarmPeers = maxSwarmPeers
	}
}

// SetActiveReservations updates the count of currently held reservation slots on this node.
func (pe *PeerExchange) SetActiveReservations(count int) {
	pe.activeReservations.Store(int32(count))
}

// PrimaryRelays returns a snapshot of known 1-hop primary relays.
func (pe *PeerExchange) PrimaryRelays() []RelayDescriptor {
	pe.relaysMu.RLock()
	defer pe.relaysMu.RUnlock()

	res := make([]RelayDescriptor, 0, len(pe.primaryRelays))
	for _, r := range pe.primaryRelays {
		res = append(res, *r)
	}
	return res
}

// BackupRelays returns a snapshot of known 2-hop backup relays.
func (pe *PeerExchange) BackupRelays() []RelayDescriptor {
	pe.relaysMu.RLock()
	defer pe.relaysMu.RUnlock()

	res := make([]RelayDescriptor, 0, len(pe.backupRelays))
	for _, r := range pe.backupRelays {
		res = append(res, *r)
	}
	return res
}

// RegisterRelay inserts or updates a relay descriptor, managing primary and 2-hop backup tables.
func (pe *PeerExchange) RegisterRelay(r RelayDescriptor) {
	if r.ID == pe.host.ID() || len(r.Addrs) == 0 {
		return
	}

	pe.relaysMu.Lock()
	defer pe.relaysMu.Unlock()

	now := time.Now().Unix()
	r.LastSeen = now

	// Clean up stale entries if primary pool is full
	if len(pe.primaryRelays) >= MaxPrimaryRelays {
		for pid, ex := range pe.primaryRelays {
			if now-ex.LastSeen > int64(RelayStaleTTL/time.Second) || ex.Failures >= 2 {
				delete(pe.primaryRelays, pid)
			}
		}
	}

	if existing, ok := pe.primaryRelays[r.ID]; ok {
		existing.Addrs = deduplicateStrings(append(existing.Addrs, r.Addrs...))
		existing.Mode = r.Mode
		existing.LoadPct = r.LoadPct
		existing.LastSeen = now
		if len(r.NextHopIDs) > 0 {
			existing.NextHopIDs = r.NextHopIDs
		}
		if len(r.NextHopAddrs) > 0 {
			existing.NextHopAddrs = r.NextHopAddrs
		}
	} else if len(pe.primaryRelays) < MaxPrimaryRelays {
		desc := r
		pe.primaryRelays[r.ID] = &desc
		delete(pe.backupRelays, r.ID)
	} else if len(pe.backupRelays) < MaxBackupRelays {
		desc := r
		pe.backupRelays[r.ID] = &desc
	}

	// Ingest 2-hop next hops into backupRelays
	for _, nextID := range r.NextHopIDs {
		if nextID == pe.host.ID() || nextID == r.ID {
			continue
		}
		if _, inPrimary := pe.primaryRelays[nextID]; inPrimary {
			continue
		}
		if _, inBackup := pe.backupRelays[nextID]; !inBackup && len(pe.backupRelays) < MaxBackupRelays {
			var addrs []string
			if r.NextHopAddrs != nil {
				addrs = r.NextHopAddrs[nextID.String()]
			}
			pe.backupRelays[nextID] = &RelayDescriptor{
				ID:       nextID,
				Addrs:    addrs,
				Mode:     "coordinator",
				LastSeen: now,
			}
		}
	}
}

// RecordRelayFailure increments failure count on a relay descriptor and evicts if threshold reached.
func (pe *PeerExchange) RecordRelayFailure(id peer.ID) {
	pe.relaysMu.Lock()
	defer pe.relaysMu.Unlock()

	if r, ok := pe.primaryRelays[id]; ok {
		r.Failures++
		if r.Failures >= 2 {
			delete(pe.primaryRelays, id)
			pe.promoteBackupsForLocked(id, r.NextHopIDs, r.NextHopAddrs)
		}
	}
	delete(pe.backupRelays, id)
}

// OnPeerDisconnected handles zero-downtime 2-hop failover promotion when a relay drops offline.
func (pe *PeerExchange) OnPeerDisconnected(id peer.ID) {
	pe.relaysMu.Lock()
	defer pe.relaysMu.Unlock()

	if r, ok := pe.primaryRelays[id]; ok {
		delete(pe.primaryRelays, id)
		pe.promoteBackupsForLocked(id, r.NextHopIDs, r.NextHopAddrs)
		slog.Info("Relay disconnected, promoted 2-hop backups to primary table",
			"disconnected_relay", id.String(),
			"promoted_count", len(r.NextHopIDs),
		)
	}
	delete(pe.backupRelays, id)

	pe.activeReservationMu.Lock()
	if pe.reservedRelayID == id {
		pe.reservedRelayID = ""
	}
	pe.activeReservationMu.Unlock()
}

func (pe *PeerExchange) promoteBackupsForLocked(failedID peer.ID, nextHopIDs []peer.ID, nextHopAddrs map[string][]string) {
	for _, hopID := range nextHopIDs {
		if hopID == pe.host.ID() || hopID == failedID {
			continue
		}
		if _, inPrimary := pe.primaryRelays[hopID]; inPrimary {
			continue
		}
		if len(pe.primaryRelays) >= MaxPrimaryRelays {
			break
		}

		if backup, ok := pe.backupRelays[hopID]; ok {
			pe.primaryRelays[hopID] = backup
			delete(pe.backupRelays, hopID)
		} else {
			var addrs []string
			if nextHopAddrs != nil {
				addrs = nextHopAddrs[hopID.String()]
			}
			pe.primaryRelays[hopID] = &RelayDescriptor{
				ID:       hopID,
				Addrs:    addrs,
				Mode:     "coordinator",
				LastSeen: time.Now().Unix(),
			}
		}
	}
}

func (pe *PeerExchange) handleStream(s network.Stream) {
	defer s.Close()

	_ = s.SetDeadline(time.Now().Add(PEXTimeout))

	var req pexRequest
	if err := json.NewDecoder(s).Decode(&req); err != nil {
		return
	}

	caller := s.Conn().RemotePeer()
	myID := pe.host.ID()

	// Ingest announced self-relay descriptor if present
	if req.SelfRelay != nil && req.SelfRelay.ID == caller {
		pe.RegisterRelay(*req.SelfRelay)
	}

	var peersList []peer.AddrInfo
	for _, pID := range pe.host.ConnectedPeers() {
		if pID == caller || pID == myID {
			continue
		}
		addrs := pe.host.raw.Peerstore().Addrs(pID)

		// Observed WAN IP extraction
		for _, c := range pe.host.raw.Network().ConnsToPeer(pID) {
			remote := c.RemoteMultiaddr()
			if ip, err := manet.ToIP(remote); err == nil && !manet.IsIPLoopback(remote) {
				if ipMA, err := manet.FromIP(ip); err == nil {
					for _, baseAddr := range pe.host.raw.Peerstore().Addrs(pID) {
						if tail := extractTransportTail(baseAddr); tail != nil {
							addrs = append(addrs, ipMA.Encapsulate(tail))
						}
					}
					addrs = append(addrs, remote)
				}
			}
		}

		// Append circuit relay fallback route through this host
		if !pe.disableRelay {
			if circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", myID)); err == nil {
				addrs = append(addrs, circuitAddr)
			}
		}

		uniqueAddrs := deduplicateMultiaddrs(addrs)
		if len(uniqueAddrs) > 0 {
			peersList = append(peersList, peer.AddrInfo{
				ID:    pID,
				Addrs: uniqueAddrs,
			})
		}
	}

	// Filter unvisited relays based on client ExcludeIDs (avoiding loops without breaking mesh cycles)
	unvisitedRelays := pe.filterUnvisitedRelays(req.ExcludeIDs, caller)

	// Dispatcher-First & 50% Soft-Cap Edge Push algorithm
	acceptReserve := pe.shouldAcceptReservation(req, unvisitedRelays)

	// Sample candidate referrals to send back to client
	sampledRelays := pe.sampleRelays(unvisitedRelays, MaxReferralSample)

	resp := pexResponse{
		Peers:         peersList,
		Relays:        sampledRelays,
		AcceptReserve: acceptReserve,
	}
	_ = json.NewEncoder(s).Encode(resp)
}

func (pe *PeerExchange) filterUnvisitedRelays(excludeIDs []peer.ID, caller peer.ID) []RelayDescriptor {
	pe.relaysMu.RLock()
	defer pe.relaysMu.RUnlock()

	excluded := make(map[peer.ID]bool, len(excludeIDs)+2)
	for _, id := range excludeIDs {
		excluded[id] = true
	}
	excluded[caller] = true
	excluded[pe.host.ID()] = true

	var unvisited []RelayDescriptor
	for _, r := range pe.primaryRelays {
		if !excluded[r.ID] && len(r.Addrs) > 0 {
			unvisited = append(unvisited, *r)
		}
	}
	return unvisited
}

func (pe *PeerExchange) shouldAcceptReservation(req pexRequest, unvisitedRelays []RelayDescriptor) bool {
	if pe.disableRelay || pe.localRelayMode == "pex-only" {
		return false
	}

	maxRes := pe.maxReservations
	if maxRes <= 0 {
		maxRes = 16
	}
	active := int(pe.activeReservations.Load())
	if active >= maxRes {
		return false
	}

	// Root seed node (Headless): Dispatcher-first. Never accepts reservations while unvisited
	// downstream relays exist, unless the client explicitly exhausted them all.
	if pe.isSeedNode {
		return len(unvisitedRelays) == 0 || req.Exhausted
	}

	// Regular public relay: 50% soft-cap edge push
	loadPct := (active * 100) / maxRes
	if loadPct < 50 {
		return true
	}

	// Load >= 50%: redirect deeper into the swarm if unvisited relays exist
	if len(unvisitedRelays) > 0 && !req.Exhausted {
		return false
	}

	// All downstream paths in cycle visited or client exhausted referrals: unlock last-50% reserve
	return true
}

func (pe *PeerExchange) sampleRelays(candidates []RelayDescriptor, maxCount int) []RelayDescriptor {
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) <= maxCount {
		return pe.attachNextHopMeta(candidates)
	}

	shuffled := make([]RelayDescriptor, len(candidates))
	copy(shuffled, candidates)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	return pe.attachNextHopMeta(shuffled[:maxCount])
}

func (pe *PeerExchange) attachNextHopMeta(relays []RelayDescriptor) []RelayDescriptor {
	pe.relaysMu.RLock()
	defer pe.relaysMu.RUnlock()

	// Gather up to MaxNextHopsPerRelay 2-hop backup IDs known by this host
	var nextHops []peer.ID
	nextAddrs := make(map[string][]string)

	for pid, r := range pe.primaryRelays {
		if len(nextHops) >= MaxNextHopsPerRelay {
			break
		}
		nextHops = append(nextHops, pid)
		nextAddrs[pid.String()] = r.Addrs
	}

	out := make([]RelayDescriptor, len(relays))
	for i, r := range relays {
		out[i] = r
		out[i].NextHopIDs = nextHops
		out[i].NextHopAddrs = nextAddrs
	}
	return out
}

// QueryPeers requests the remote peer's connected swarm peer list and attempts connections.
func (pe *PeerExchange) QueryPeers(ctx context.Context, target peer.ID) ([]peer.ID, error) {
	if target == pe.host.ID() {
		return nil, nil
	}

	s, err := pe.host.NewControlStream(ctx, target, PEXProtocolID)
	if err != nil {
		return nil, fmt.Errorf("pex: failed to open stream to %s: %w", target, err)
	}
	defer s.Close()

	_ = s.SetDeadline(time.Now().Add(PEXTimeout))

	req := pexRequest{
		Type: "get_peers",
	}
	if pe.host.IsRelayServiceActive() {
		myAddrs := make([]string, 0, len(pe.host.Addrs()))
		for _, a := range pe.host.Addrs() {
			myAddrs = append(myAddrs, a.String())
		}
		req.SelfRelay = &RelayDescriptor{
			ID:      pe.host.ID(),
			Addrs:   myAddrs,
			Mode:    pe.localRelayMode,
			LoadPct: (int(pe.activeReservations.Load()) * 100) / max(1, pe.maxReservations),
		}
	}

	if err := json.NewEncoder(s).Encode(req); err != nil {
		return nil, fmt.Errorf("pex: failed to send request: %w", err)
	}

	var resp pexResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		return nil, fmt.Errorf("pex: failed to decode response: %w", err)
	}

	// Ingest discovered relays
	for _, r := range resp.Relays {
		pe.RegisterRelay(r)
	}

	var connected []peer.ID
	for _, pi := range resp.Peers {
		if pi.ID == pe.host.ID() {
			continue
		}

		// Record the 2-hop referral in the bounded peer tracker before attempting dial
		if pe.tracker != nil {
			pe.tracker.RecordPeerVia(pi, len(pe.host.raw.Network().ConnsToPeer(pi.ID)) > 0, target)
		}

		if len(pe.host.raw.Network().ConnsToPeer(pi.ID)) > 0 {
			continue
		}

		if pe.maxSwarmPeers > 0 && len(pe.host.ConnectedPeers()) >= pe.maxSwarmPeers {
			continue
		}

		if !pe.disableRelay {
			if circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", target)); err == nil {
				pi.Addrs = append(pi.Addrs, circuitAddr)
			}
		}
		pi.Addrs = deduplicateMultiaddrs(pi.Addrs)

		dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := pe.host.Connect(dialCtx, pi)
		cancel()

		if err != nil {
			slog.Debug("PEX auto-dial failed", "peer_id", pi.ID.String(), "error", err)
		} else {
			slog.Info("Successfully connected to peer discovered via PEX", "peer_id", pi.ID.String())
			connected = append(connected, pi.ID)
			if pe.tracker != nil {
				pe.tracker.RecordPeerVia(pi, true, target)
			}
		}
	}

	return connected, nil
}

// FindAndReserveRelay performs cycle-free, probabilistic deep traversal to secure a circuit relay slot.
func (pe *PeerExchange) FindAndReserveRelay(ctx context.Context, seedIDs []peer.ID) (peer.ID, error) {
	if pe.disableRelay {
		return "", errors.New("pex: relay support is disabled")
	}

	visited := make(map[peer.ID]bool)
	visited[pe.host.ID()] = true

	// Build initial candidate queue: community primary relays first, backups second, seeds last
	queue := make([]peer.ID, 0, len(pe.primaryRelays)+len(seedIDs))
	seedSet := make(map[peer.ID]bool, len(seedIDs))
	for _, s := range seedIDs {
		seedSet[s] = true
	}

	pe.relaysMu.RLock()
	for pid := range pe.primaryRelays {
		if !seedSet[pid] {
			queue = append(queue, pid)
		}
	}
	for pid := range pe.backupRelays {
		if !seedSet[pid] {
			queue = append(queue, pid)
		}
	}
	pe.relaysMu.RUnlock()

	// Append seeds at the back of the queue (render seed is last resort)
	queue = append(queue, seedIDs...)

	var lastReachable peer.ID

	for len(queue) > 0 {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		candID := queue[0]
		queue = queue[1:]

		if visited[candID] {
			continue
		}
		visited[candID] = true

		excludeSlice := make([]peer.ID, 0, len(visited))
		for v := range visited {
			excludeSlice = append(excludeSlice, v)
		}

		s, err := pe.host.NewControlStream(ctx, candID, PEXProtocolID)
		if err != nil {
			pe.RecordRelayFailure(candID)
			continue
		}

		_ = s.SetDeadline(time.Now().Add(PEXTimeout))
		req := pexRequest{
			Type:       "find_relay",
			ExcludeIDs: excludeSlice,
			Exhausted:  false,
		}
		if err := json.NewEncoder(s).Encode(req); err != nil {
			s.Close()
			pe.RecordRelayFailure(candID)
			continue
		}

		var resp pexResponse
		if err := json.NewDecoder(s).Decode(&resp); err != nil {
			s.Close()
			pe.RecordRelayFailure(candID)
			continue
		}
		s.Close()

		lastReachable = candID

		// Ingest returned relays into local registry
		for _, r := range resp.Relays {
			pe.RegisterRelay(r)
			if !visited[r.ID] {
				// Prepend unvisited referrals to front of queue (Edge Push exploration)
				queue = append([]peer.ID{r.ID}, queue...)
			}
		}

		if resp.AcceptReserve {
			resCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			res, err := pe.host.ReserveRelay(resCtx, candID)
			cancel()
			if err == nil && res != nil {
				pe.activeReservationMu.Lock()
				pe.reservedRelayID = candID
				pe.activeReservationMu.Unlock()
				slog.Info("Successfully acquired circuit relay reservation", "relay_peer", candID.String())
				return candID, nil
			}
			pe.RecordRelayFailure(candID)
		}
	}

	// Emergency fallback pass: query last reachable relay with Exhausted: true
	if lastReachable != "" {
		s, err := pe.host.NewControlStream(ctx, lastReachable, PEXProtocolID)
		if err == nil {
			defer s.Close()
			_ = s.SetDeadline(time.Now().Add(PEXTimeout))
			req := pexRequest{
				Type:      "find_relay",
				Exhausted: true,
			}
			if err := json.NewEncoder(s).Encode(req); err == nil {
				var resp pexResponse
				if err := json.NewDecoder(s).Decode(&resp); err == nil && resp.AcceptReserve {
					resCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
					res, err := pe.host.ReserveRelay(resCtx, lastReachable)
					cancel()
					if err == nil && res != nil {
						pe.activeReservationMu.Lock()
						pe.reservedRelayID = lastReachable
						pe.activeReservationMu.Unlock()
						slog.Info("Acquired emergency circuit relay reservation", "relay_peer", lastReachable.String())
						return lastReachable, nil
					}
				}
			}
		}
	}

	return "", ErrNoRelayAvailable
}

// StartRelayOrchestrator maintains 1 active relay reservation in the background, prioritizing community relays.
func (pe *PeerExchange) StartRelayOrchestrator(ctx context.Context, bootstrapPeerIDs []peer.ID) {
	if pe.disableRelay {
		return
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		// Initial reservation attempt after startup
		time.Sleep(2 * time.Second)
		if pe.needsReservation() {
			reserveCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			_, _ = pe.FindAndReserveRelay(reserveCtx, bootstrapPeerIDs)
			cancel()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if pe.needsReservation() {
					reserveCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
					_, _ = pe.FindAndReserveRelay(reserveCtx, bootstrapPeerIDs)
					cancel()
				}
			}
		}
	}()
}

func (pe *PeerExchange) needsReservation() bool {
	// If host is running its own relay service (public reachability), it does not need a client reservation
	if pe.host.IsRelayServiceActive() {
		return false
	}
	pe.activeReservationMu.Lock()
	defer pe.activeReservationMu.Unlock()
	return pe.reservedRelayID == ""
}

func extractTransportTail(a multiaddr.Multiaddr) multiaddr.Multiaddr {
	comps := multiaddr.Split(a)
	if len(comps) <= 1 {
		return nil
	}
	if comps[0].Protocol().Code != multiaddr.P_IP4 && comps[0].Protocol().Code != multiaddr.P_IP6 {
		return nil
	}
	var tail []multiaddr.Multiaddrer
	for i := 1; i < len(comps); i++ {
		tail = append(tail, &comps[i])
	}
	return multiaddr.Join(tail...)
}

func deduplicateMultiaddrs(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
	seen := make(map[string]bool, len(addrs))
	var out []multiaddr.Multiaddr
	for _, a := range addrs {
		if a == nil {
			continue
		}
		s := a.String()
		if !seen[s] {
			seen[s] = true
			out = append(out, a)
		}
	}
	return out
}

func deduplicateStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	var out []string
	for _, s := range items {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
