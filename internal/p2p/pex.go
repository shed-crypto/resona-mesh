package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
)

const (
	PEXProtocolID = protocol.ID("/resonamesh/pex/1.0.0")
	PEXTimeout    = 8 * time.Second
)

type pexRequest struct {
	Type string `json:"type"`
}

type pexResponse struct {
	Peers []peer.AddrInfo `json:"peers"`
}

// PeerExchange coordinates active swarm peer discovery through connected peers and relays.
type PeerExchange struct {
	host         *Host
	tracker      *PeerTracker
	disableRelay bool
}

// NewPeerExchange creates and registers the PEX wire protocol on the host.
func NewPeerExchange(h *Host, tracker *PeerTracker) *PeerExchange {
	pe := &PeerExchange{
		host:    h,
		tracker: tracker,
	}
	h.raw.SetStreamHandler(PEXProtocolID, pe.handleStream)
	return pe
}

// SetDisableRelay controls whether circuit relay addresses are appended in PEX.
func (pe *PeerExchange) SetDisableRelay(disabled bool) {
	pe.disableRelay = disabled
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

	var peersList []peer.AddrInfo
	for _, pID := range pe.host.ConnectedPeers() {
		if pID == caller || pID == myID {
			continue
		}
		addrs := pe.host.raw.Peerstore().Addrs(pID)

		// Observed WAN IP extraction:
		// Extract observed remote IP from active connections to this peer (similar to BitTorrent tracker observed IP)
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

		// Append circuit relay fallback route through this host so remote NAT peers can traverse
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

	resp := pexResponse{Peers: peersList}
	_ = json.NewEncoder(s).Encode(resp)
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

	if err := json.NewEncoder(s).Encode(pexRequest{Type: "get_peers"}); err != nil {
		return nil, fmt.Errorf("pex: failed to send request: %w", err)
	}

	var resp pexResponse
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		return nil, fmt.Errorf("pex: failed to decode response: %w", err)
	}

	var connected []peer.ID
	for _, pi := range resp.Peers {
		if pi.ID == pe.host.ID() || len(pe.host.raw.Network().ConnsToPeer(pi.ID)) > 0 {
			continue
		}

		// Ensure circuit relay fallback through target is present for NAT traversal unless disabled
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
				pe.tracker.RecordPeer(pi, true)
			}
		}
	}

	return connected, nil
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
