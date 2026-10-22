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
	host    *Host
	tracker *PeerTracker
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
		if len(addrs) > 0 {
			// Append circuit relay fallback route through this host so remote NAT peers can traverse
			if circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", myID)); err == nil {
				addrs = append(addrs, circuitAddr)
			}
			peersList = append(peersList, peer.AddrInfo{
				ID:    pID,
				Addrs: addrs,
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

	s, err := pe.host.raw.NewStream(ctx, target, PEXProtocolID)
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

		// Always ensure circuit relay fallback through target is present for NAT traversal
		if circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", target)); err == nil {
			pi.Addrs = append(pi.Addrs, circuitAddr)
		}

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
