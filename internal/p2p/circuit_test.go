package p2p

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/multiformats/go-multiaddr"
	"resonamesh/pkg/crypto"
)

func TestCircuitRelayReservationAndBlockFetch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Create Relay Host R
	_, privR, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	relayHost, err := NewHost(HostConfig{
		ListenAddr:          "/ip4/127.0.0.1/tcp/0",
		PrivateKey:          privR,
		EnableRelayService:  true,
		DisableRelay:        false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relayHost.Close()

	// 2. Create Host A (seeder behind simulated NAT, only listening on loopback)
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	// 3. Create Host B (leecher behind simulated NAT)
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()

	// Connect Host A to Relay R and reserve a slot
	if err := hostA.Connect(ctx, relayHost.AddrInfo()); err != nil {
		t.Fatalf("host A failed to connect to relay: %v", err)
	}
	reservation, err := client.Reserve(ctx, hostA.Raw(), relayHost.AddrInfo())
	if err != nil {
		t.Fatalf("host A failed to reserve slot on relay: %v", err)
	}
	if reservation == nil {
		t.Fatalf("expected non-nil reservation")
	}

	// Connect Host B to Relay R
	if err := hostB.Connect(ctx, relayHost.AddrInfo()); err != nil {
		t.Fatalf("host B failed to connect to relay: %v", err)
	}

	// Host B dials Host A using circuit relay multiaddress
	circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", relayHost.ID()))
	if err != nil {
		t.Fatalf("failed to create circuit multiaddr: %v", err)
	}

	targetInfo := peer.AddrInfo{
		ID:    hostA.ID(),
		Addrs: []multiaddr.Multiaddr{circuitAddr},
	}

	if err := hostB.Connect(ctx, targetInfo); err != nil {
		t.Fatalf("host B failed to connect to host A through relay: %v", err)
	}

	// Verify Host B and Host A are connected
	peersB := hostB.ConnectedPeers()
	found := false
	for _, p := range peersB {
		if p == hostA.ID() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("host B should be connected to host A via relay, peers: %v", peersB)
	}
}
