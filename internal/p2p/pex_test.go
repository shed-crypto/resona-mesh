package p2p

import (
	"context"
	"testing"
	"time"

	"resonamesh/pkg/crypto"

	"github.com/multiformats/go-multiaddr"
)

func TestPeerExchangeDiscovery(t *testing.T) {
	_, privRelay, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	// 1. Start Relay Node with PEX enabled
	hostRelay, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privRelay,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostRelay.Close()
	_ = NewPeerExchange(hostRelay, nil)

	// 2. Start Client A and connect to Relay
	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()
	_ = NewPeerExchange(hostA, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostRelay.AddrInfo()); err != nil {
		t.Fatalf("failed to connect Client A to Relay: %v", err)
	}

	// 3. Start Client B with PEX and connect to Relay
	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()
	trackerB := NewPeerTracker()
	pexB := NewPeerExchange(hostB, trackerB)

	if err := hostB.Connect(ctx, hostRelay.AddrInfo()); err != nil {
		t.Fatalf("failed to connect Client B to Relay: %v", err)
	}

	// 4. Client B queries Relay for other swarm peers
	connected, err := pexB.QueryPeers(ctx, hostRelay.ID())
	if err != nil {
		t.Fatalf("pex query failed: %v", err)
	}

	if len(connected) != 1 || connected[0] != hostA.ID() {
		t.Fatalf("expected Client B to discover and connect to Client A %s, got: %v", hostA.ID(), connected)
	}

	// Verify Client B and Client A are directly connected
	peersB := hostB.ConnectedPeers()
	hasClientA := false
	for _, p := range peersB {
		if p == hostA.ID() {
			hasClientA = true
			break
		}
	}
	if !hasClientA {
		t.Fatalf("expected Client B to have active connection with Client A, got peers: %v", peersB)
	}
}

func TestTransportTailExtractionAndDeduplication(t *testing.T) {
	tcpAddr, err := multiaddr.NewMultiaddr("/ip4/192.168.1.10/tcp/4001")
	if err != nil {
		t.Fatal(err)
	}
	quicAddr, err := multiaddr.NewMultiaddr("/ip4/10.0.0.5/udp/4001/quic-v1")
	if err != nil {
		t.Fatal(err)
	}

	tcpTail := extractTransportTail(tcpAddr)
	if tcpTail == nil || tcpTail.String() != "/tcp/4001" {
		t.Fatalf("expected /tcp/4001, got %v", tcpTail)
	}

	quicTail := extractTransportTail(quicAddr)
	if quicTail == nil || quicTail.String() != "/udp/4001/quic-v1" {
		t.Fatalf("expected /udp/4001/quic-v1, got %v", quicTail)
	}

	publicIP, err := multiaddr.NewMultiaddr("/ip4/93.180.12.34")
	if err != nil {
		t.Fatal(err)
	}

	synthesizedTCP := publicIP.Encapsulate(tcpTail)
	if synthesizedTCP.String() != "/ip4/93.180.12.34/tcp/4001" {
		t.Fatalf("expected synthesized TCP address, got %s", synthesizedTCP.String())
	}

	synthesizedQUIC := publicIP.Encapsulate(quicTail)
	if synthesizedQUIC.String() != "/ip4/93.180.12.34/udp/4001/quic-v1" {
		t.Fatalf("expected synthesized QUIC address, got %s", synthesizedQUIC.String())
	}

	// Test deduplication
	dupList := []multiaddr.Multiaddr{
		synthesizedTCP,
		synthesizedTCP,
		synthesizedQUIC,
		synthesizedQUIC,
	}
	deduped := deduplicateMultiaddrs(dupList)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 unique addresses, got %d", len(deduped))
	}
}
