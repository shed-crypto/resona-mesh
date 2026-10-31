package p2p

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/connmgr"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/transport"
	circuitclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	circuitrelay "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/libp2p/go-libp2p/p2p/protocol/holepunch"
	"github.com/libp2p/go-libp2p/p2p/security/noise"
	libp2pquic "github.com/libp2p/go-libp2p/p2p/transport/quic"
	"github.com/libp2p/go-libp2p/p2p/transport/quicreuse"
	libp2ptcp "github.com/libp2p/go-libp2p/p2p/transport/tcp"
	libp2pws "github.com/libp2p/go-libp2p/p2p/transport/websocket"
	"github.com/multiformats/go-multiaddr"
)

var (
	ErrNilPrivateKey   = errors.New("p2p: private key is required")
	ErrEmptyListenAddr = errors.New("p2p: listen address cannot be empty")
	ErrNoQUICListener  = errors.New("p2p: no active QUIC UDP listener available for shared STUN")
)

// DefaultDirectUpgradeWait is the time spent waiting for DCUtR / direct QUIC upgrade
// before falling back to circuit relay for heavy audio block streams.
const DefaultDirectUpgradeWait = 2500 * time.Millisecond

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
	quicConnMgr  *quicreuse.ConnManager
	disableRelay bool
	extraAddrsMu sync.RWMutex
	extraAddrs   []multiaddr.Multiaddr
	stunAddrs    []multiaddr.Multiaddr
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

	h := &Host{
		disableRelay: cfg.DisableRelay,
	}

	opts := []libp2p.Option{
		libp2p.Identity(p2pPrivKey),
		libp2p.ListenAddrStrings(listenAddrs...),
		libp2p.Transport(libp2ptcp.NewTCPTransport),
		libp2p.Transport(func(key libp2pcrypto.PrivKey, connManager *quicreuse.ConnManager, psk pnet.PSK, gater connmgr.ConnectionGater, rcmgr network.ResourceManager) (transport.Transport, error) {
			h.quicConnMgr = connManager
			return libp2pquic.NewTransport(key, connManager, psk, gater, rcmgr)
		}),
		libp2p.Security(noise.ID, noise.New),
		libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			h.extraAddrsMu.RLock()
			defer h.extraAddrsMu.RUnlock()
			if len(h.extraAddrs) == 0 && len(h.stunAddrs) == 0 {
				return addrs
			}
			res := make([]multiaddr.Multiaddr, len(addrs), len(addrs)+len(h.stunAddrs)+len(h.extraAddrs))
			copy(res, addrs)
			res = append(res, h.stunAddrs...)
			res = append(res, h.extraAddrs...)
			return deduplicateMultiaddrs(res)
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
		opts = append(opts, libp2p.EnableRelay(), libp2p.EnableHolePunching(
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

// HasDirectConnection reports whether there is at least one non-relayed, unlimited connection to peerID.
func (h *Host) HasDirectConnection(peerID peer.ID) bool {
	for _, c := range h.raw.Network().ConnsToPeer(peerID) {
		if !c.Stat().Limited && !strings.Contains(c.RemoteMultiaddr().String(), "p2p-circuit") {
			return true
		}
	}
	return false
}

// NewControlStream opens a lightweight control/metadata stream (CatalogSync, PEX, Moderation),
// permitting use of a circuit relay connection when a direct connection is not yet established.
func (h *Host) NewControlStream(ctx context.Context, peerID peer.ID, pids ...protocol.ID) (network.Stream, error) {
	allowCtx := network.WithAllowLimitedConn(ctx, "control-plane")
	return h.raw.NewStream(allowCtx, peerID, pids...)
}

// NewDataStreamPreferDirect opens a data stream (such as audio block transfer) by strongly prioritizing
// a direct P2P connection. If the peer is currently only connected via a circuit relay, it triggers
// a direct dial / DCUtR hole-punch wait for up to directWait before falling back to the relay as a last resort.
func (h *Host) NewDataStreamPreferDirect(ctx context.Context, peerID peer.ID, directWait time.Duration, pids ...protocol.ID) (network.Stream, error) {
	if h.HasDirectConnection(peerID) {
		return h.raw.NewStream(ctx, peerID, pids...)
	}

	if directWait <= 0 {
		directWait = DefaultDirectUpgradeWait
	}

	// Attempt to upgrade from relay to direct connection first so audio payloads bypass the relay server
	waitCtx, cancel := context.WithTimeout(ctx, directWait)
	defer cancel()

	// Attempt direct dial first using known direct multiaddrs in peerstore
	h.tryForceDirectDial(waitCtx, peerID)

	if s, err := h.raw.NewStream(waitCtx, peerID, pids...); err == nil {
		return s, nil
	}

	// Last resort: if direct hole punch failed/timed out, allow limited relay stream so playback does not break
	slog.Info("Direct P2P upgrade unavailable, falling back to circuit relay as last resort", "peer_id", peerID.String())
	relayCtx := network.WithAllowLimitedConn(ctx, "relay-data-fallback")
	return h.raw.NewStream(relayCtx, peerID, pids...)
}

func (h *Host) tryForceDirectDial(ctx context.Context, peerID peer.ID) {
	if h.HasDirectConnection(peerID) {
		return
	}
	var directAddrs []multiaddr.Multiaddr
	for _, a := range h.raw.Peerstore().Addrs(peerID) {
		if a != nil && !strings.Contains(a.String(), "p2p-circuit") {
			directAddrs = append(directAddrs, a)
		}
	}
	if len(directAddrs) == 0 {
		return
	}
	forceCtx := network.WithForceDirectDial(ctx, "dcutr-upgrade")
	_ = h.raw.Connect(forceCtx, peer.AddrInfo{
		ID:    peerID,
		Addrs: directAddrs,
	})
}

// SharedUDPPacketConn returns a net.PacketConn sharing the exact UDP socket bound by the host's
// IPv4 QUIC-v1 listener, along with the local bound UDP port. Closing the returned PacketConn
// only decrements the refcount without closing the underlying QUIC listener.
func (h *Host) SharedUDPPacketConn() (net.PacketConn, int, error) {
	if h.quicConnMgr == nil {
		return nil, 0, ErrNoQUICListener
	}

	// Prefer wildcard 0.0.0.0 listener if bound, otherwise fall back to first IPv4 QUIC listener (e.g. 127.0.0.1 in tests)
	var candidateIP net.IP
	var candidatePort int

	for _, ma := range h.raw.Network().ListenAddresses() {
		if _, err := ma.ValueForProtocol(multiaddr.P_QUIC_V1); err != nil {
			continue
		}
		ipStr, err := ma.ValueForProtocol(multiaddr.P_IP4)
		if err != nil {
			continue
		}
		portStr, err := ma.ValueForProtocol(multiaddr.P_UDP)
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 {
			continue
		}
		ip := net.ParseIP(ipStr)
		if ip == nil {
			continue
		}

		// Try wildcard 0.0.0.0 first since ListenAddrStrings("/ip4/0.0.0.0/udp/...") registers under 0.0.0.0
		if conn, err := h.quicConnMgr.SharedNonQUICPacketConn("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: port}); err == nil {
			return conn, port, nil
		}
		if conn, err := h.quicConnMgr.SharedNonQUICPacketConn("udp4", &net.UDPAddr{IP: ip, Port: port}); err == nil {
			return conn, port, nil
		}
		candidateIP = ip
		candidatePort = port
	}

	if candidatePort > 0 && candidateIP != nil {
		conn, err := h.quicConnMgr.SharedNonQUICPacketConn("udp4", &net.UDPAddr{IP: candidateIP, Port: candidatePort})
		if err == nil {
			return conn, candidatePort, nil
		}
	}

	return nil, 0, ErrNoQUICListener
}

// ProbeAndAdvertiseNAT performs multi-server STUN diagnosis over the host's shared QUIC UDP socket
// and updates the host's advertised public QUIC (and predicted symmetric NAT) multiaddresses.
func (h *Host) ProbeAndAdvertiseNAT(ctx context.Context, servers []string) (*NATDiagnosis, error) {
	pc, localPort, err := h.SharedUDPPacketConn()
	if err == nil && pc != nil {
		defer pc.Close()
	}

	diag, err := DiagnoseNAT(ctx, pc, servers)
	if err != nil {
		return nil, err
	}

	ipProto := "ip4"
	if diag.PublicIP.To4() == nil {
		ipProto = "ip6"
	}

	var newSTUNAddrs []multiaddr.Multiaddr
	for _, port := range diag.CandidatePorts {
		if quicMA, err := multiaddr.NewMultiaddr(fmt.Sprintf("/%s/%s/udp/%d/quic-v1", ipProto, diag.PublicIP.String(), port)); err == nil {
			newSTUNAddrs = append(newSTUNAddrs, quicMA)
		}
	}

	tcpPort := diag.PrimaryPort
	if localPort > 0 {
		tcpPort = localPort
	}
	if tcpMA, err := multiaddr.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipProto, diag.PublicIP.String(), tcpPort)); err == nil {
		newSTUNAddrs = append(newSTUNAddrs, tcpMA)
	}

	h.SetSTUNAddrs(newSTUNAddrs)
	return diag, nil
}

// StartSTUNKeepAlive periodically refreshes the STUN binding on the shared QUIC UDP socket
// so the NAT router's UDP pinhole remains open for incoming DCUtR hole punching.
func (h *Host) StartSTUNKeepAlive(ctx context.Context, servers []string, interval time.Duration) {
	if interval <= 0 {
		interval = 25 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, _ = h.ProbeAndAdvertiseNAT(probeCtx, servers)
				cancel()
			}
		}
	}()
}

// SetSTUNAddrs atomically replaces the STUN-discovered public multiaddresses advertised by this host.
func (h *Host) SetSTUNAddrs(addrs []multiaddr.Multiaddr) {
	h.extraAddrsMu.Lock()
	defer h.extraAddrsMu.Unlock()
	h.stunAddrs = deduplicateMultiaddrs(addrs)
}

// ReserveRelay attempts to reserve a circuit relay v2 slot on the specified relay peer
// and advertises the resulting /p2p-circuit address for inbound NAT hole punching coordination.
func (h *Host) ReserveRelay(ctx context.Context, relayID peer.ID) (*circuitclient.Reservation, error) {
	res, err := circuitclient.Reserve(ctx, h.raw, peer.AddrInfo{ID: relayID})
	if err != nil {
		return nil, err
	}
	if circuitMA, maErr := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", relayID)); maErr == nil {
		h.AddAdvertisedAddr(circuitMA)
	}
	return res, nil
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
