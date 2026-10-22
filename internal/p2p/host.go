package p2p

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/transport"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2ptcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	libp2pws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	circuitclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	circuitrelay "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/multiformats/go-multiaddr"
)

var (
	ErrNilPrivateKey = errors.New("p2p: private key is required")
	ErrEmptyListenAddr = errors.New("p2p: listen address cannot be empty")
)

// HostConfig holds networking configuration for the libp2p host.
type HostConfig struct {
	ListenAddr         string
	ListenAddrs        []string
	PrivateKey         ed25519.PrivateKey
	DisableRelay       bool
	EnableRelayService bool
	EnableNATPortMap   bool
	HTTPHandler        http.Handler
}

// Host wraps the libp2p Host instance providing lifecycle and identity operations.
type Host struct {
	raw          host.Host
	extraAddrsMu sync.RWMutex
	extraAddrs   []multiaddr.Multiaddr
}

// NewHost creates and initializes a libp2p host bound to the node identity key.
func NewHost(cfg HostConfig) (*Host, error) {
	if len(cfg.PrivateKey) == 0 {
		return nil, ErrNilPrivateKey
	}

	var listenAddrs []string
	if len(cfg.ListenAddrs) > 0 {
		listenAddrs = cfg.ListenAddrs
	} else if cfg.ListenAddr != "" {
		listenAddrs = []string{cfg.ListenAddr}
	} else {
		return nil, ErrEmptyListenAddr
	}

	p2pPrivKey, err := libp2pcrypto.UnmarshalEd25519PrivateKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to unmarshal ed25519 private key: %w", err)
	}

	h := &Host{}

	opts := []libp2p.Option{
		libp2p.Identity(p2pPrivKey),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.Transport(libp2ptcp.NewTCPTransport),
		libp2p.Transport(libp2pquic.NewTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			h.extraAddrsMu.RLock()
			defer h.extraAddrsMu.RUnlock()
			if len(h.extraAddrs) == 0 {
				return addrs
			}
			res := make([]multiaddr.Multiaddr, len(addrs), len(addrs)+len(h.extraAddrs))
			copy(res, addrs)
			return append(res, h.extraAddrs...)
		}),
	}

	if cfg.HTTPHandler != nil {
		opts = append(opts, libp2p.Transport(func(u transport.Upgrader, rcmgr network.ResourceManager) (transport.Transport, error) {
			return libp2pws.New(u, rcmgr, nil, libp2pws.WithHTTPHandler(cfg.HTTPHandler))
		}))
	} else {
		opts = append(opts, libp2p.Transport(libp2pws.New))
	}

	if cfg.DisableRelay {
		opts = append(opts, libp2p.DisableRelay())
	} else {
		opts = append(opts, libp2p.EnableHolePunching(
			holepunch.WithTracer(&holePunchLogger{}),
		))
	}
	if cfg.EnableNATPortMap {
		opts = append(opts, libp2p.NATPortMap())
	}
	if cfg.EnableRelayService {
		opts = append(opts, libp2p.EnableNATService(), libp2p.ForceReachabilityPublic())
	}

	rawHost, err := libp2p.New(opts...)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to construct libp2p host: %w", err)
	}

	if cfg.EnableRelayService {
		if _, err := circuitrelay.New(rawHost, circuitrelay.WithInfiniteLimits()); err != nil {
			slog.Warn("Failed to initialize circuit relay v2 service", "error", err)
		} else {
			slog.Info("Circuit relay v2 service active on host with infinite limits")
		}
	}

	slog.Info("Initialized libp2p host",
		"peer_id", rawHost.ID().String(),
		"listen_addrs", rawHost.Addrs(),
	)

	h.raw = rawHost
	return h, nil
}

// ID returns the libp2p Peer ID derived from the node's public key.
func (h *Host) ID() peer.ID {
	return h.raw.ID()
}

// Addrs returns all listen multiaddresses assigned to this host.
func (h *Host) Addrs() []multiaddr.Multiaddr {
	return h.raw.Addrs()
}

// AddrInfo returns the peer.AddrInfo descriptor of this host.
func (h *Host) AddrInfo() peer.AddrInfo {
	return peer.AddrInfo{
		ID:    h.raw.ID(),
		Addrs: h.raw.Addrs(),
	}
}

// Raw returns the underlying libp2p host.Host.
func (h *Host) Raw() host.Host {
	return h.raw
}

// Network returns the underlying libp2p network.Network.
func (h *Host) Network() network.Network {
	return h.raw.Network()
}

// Connect dials and establishes an authenticated connection to a remote peer.
func (h *Host) Connect(ctx context.Context, pi peer.AddrInfo) error {
	if err := h.raw.Connect(ctx, pi); err != nil {
		return fmt.Errorf("p2p: failed to connect to peer %s: %w", pi.ID, err)
	}
	return nil
}

// ConnectedPeers returns a slice of all currently connected peer IDs.
func (h *Host) ConnectedPeers() []peer.ID {
	return h.raw.Network().Peers()
}

// ReserveRelay attempts to reserve a circuit relay v2 slot on the specified relay peer.
func (h *Host) ReserveRelay(ctx context.Context, relayID peer.ID) (*circuitclient.Reservation, error) {
	return circuitclient.Reserve(ctx, h.raw, peer.AddrInfo{ID: relayID})
}

// AddAdvertisedAddr dynamically appends a discovered public reflexive address (e.g. from STUN or UPnP).
func (h *Host) AddAdvertisedAddr(ma multiaddr.Multiaddr) {
	if ma == nil {
		return
	}
	h.extraAddrsMu.Lock()
	defer h.extraAddrsMu.Unlock()
	for _, existing := range h.extraAddrs {
		if existing.Equal(ma) {
			return
		}
	}
	h.extraAddrs = append(h.extraAddrs, ma)
}

// Close gracefully closes the host, terminating active connections and streams.
func (h *Host) Close() error {
	return h.raw.Close()
}

type holePunchLogger struct{}

func (l *holePunchLogger) Trace(e *holepunch.Event) {
	switch v := e.Evt.(type) {
	case *holepunch.StartHolePunchEvt:
		slog.Info("NAT DCUtR hole punching initiated",
			"remote_peer", e.Remote.String(),
			"remote_addrs", v.RemoteAddrs,
			"rtt_ms", v.RTT.Milliseconds(),
		)
	case *holepunch.DirectDialEvt:
		if v.Success {
			slog.Info("NAT DCUtR direct dial succeeded",
				"remote_peer", e.Remote.String(),
				"elapsed_ms", v.EllapsedTime.Milliseconds(),
			)
		} else {
			slog.Debug("NAT DCUtR direct dial attempt failed",
				"remote_peer", e.Remote.String(),
				"elapsed_ms", v.EllapsedTime.Milliseconds(),
				"error", v.Error,
			)
		}
	case *holepunch.EndHolePunchEvt:
		if v.Success {
			slog.Info("NAT DCUtR hole punching successfully upgraded connection to DIRECT P2P",
				"remote_peer", e.Remote.String(),
				"elapsed_ms", v.EllapsedTime.Milliseconds(),
			)
		} else {
			slog.Warn("NAT DCUtR hole punching failed, remaining on circuit relay fallback",
				"remote_peer", e.Remote.String(),
				"elapsed_ms", v.EllapsedTime.Milliseconds(),
				"error", v.Error,
			)
		}
	case *holepunch.ProtocolErrorEvt:
		slog.Debug("NAT DCUtR protocol error",
			"remote_peer", e.Remote.String(),
			"type", e.Type,
		)
	}
}

