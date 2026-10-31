package p2p

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
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
		ListenAddr:         "/ip4/127.0.0.1/tcp/0",
		PrivateKey:         privR,
		EnableRelayService: true,
		DisableRelay:       false,
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

func TestRelayToDirectUpgradeAndZeroRelayAudioTransfer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Relay R
	_, privR, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	relayHost, err := NewHost(HostConfig{
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		PrivateKey:         privR,
		EnableRelayService: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relayHost.Close()

	// 2. Seeder Host A (with QUIC-v1 enabled)
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hostA, err := NewHost(HostConfig{
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		PrivateKey: privA,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	// Register audio blocks on Seeder A
	chunk0 := bytes.Repeat([]byte("NEON-MELODY-CHUNK-0-"), 1024)
	chunk1 := bytes.Repeat([]byte("NEON-MELODY-CHUNK-1-"), 1024)
	hash0 := crypto.HashChunkHex(chunk0)
	hash1 := crypto.HashChunkHex(chunk1)

	NewProtocolHandler(hostA, &memoryProvider{
		blocks: map[string][]byte{
			hash0: chunk0,
			hash1: chunk1,
		},
	})

	// 3. Leecher Host B (with QUIC-v1 enabled)
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	hostB, err := NewHost(HostConfig{
		ListenAddrs: []string{
			"/ip4/127.0.0.1/tcp/0",
			"/ip4/127.0.0.1/udp/0/quic-v1",
		},
		PrivateKey: privB,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()

	// Both A and B connect ONLY to Relay R
	if err := hostA.Connect(ctx, relayHost.AddrInfo()); err != nil {
		t.Fatalf("host A connect to relay: %v", err)
	}
	if _, err := hostA.ReserveRelay(ctx, relayHost.ID()); err != nil {
		t.Fatalf("host A reserve relay: %v", err)
	}
	if err := hostB.Connect(ctx, relayHost.AddrInfo()); err != nil {
		t.Fatalf("host B connect to relay: %v", err)
	}

	// Initially, Leecher B dials Seeder A ONLY via /p2p-circuit (no direct address known yet)
	circuitAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", relayHost.ID()))
	if err != nil {
		t.Fatal(err)
	}
	if err := hostB.Connect(ctx, peer.AddrInfo{
		ID:    hostA.ID(),
		Addrs: []multiaddr.Multiaddr{circuitAddr},
	}); err != nil {
		t.Fatalf("host B failed initial circuit connect to host A: %v", err)
	}

	if hostB.HasDirectConnection(hostA.ID()) {
		t.Fatalf("expected initial connection between B and A to be relayed only, not direct")
	}

	// Simulate STUN/Identify exchanging Seeder A's QUIC endpoint over the relay channel
	hostB.Raw().Peerstore().AddAddrs(hostA.ID(), hostA.Addrs(), peerstore.PermanentAddrTTL)

	// Leecher B requests Block 0: NewDataStreamPreferDirect must upgrade to DIRECT P2P before streaming
	got0, err := RequestBlock(ctx, hostB, hostA.ID(), WireBlockRequest{
		ContentID:  "track-direct-upgrade",
		BlockIndex: 0,
		BlockHash:  hash0,
	})
	if err != nil {
		t.Fatalf("RequestBlock 0 failed: %v", err)
	}
	if !bytes.Equal(got0, chunk0) {
		t.Fatalf("chunk 0 payload mismatch")
	}

	// Verify connection was upgraded to DIRECT P2P
	if !hostB.HasDirectConnection(hostA.ID()) {
		t.Fatalf("expected connection between B and A to be upgraded to DIRECT P2P")
	}

	// Now completely shut down Relay R!
	// If the melody is truly streaming directly peer-to-peer without the relay server,
	// fetching the rest of the melody (Block 1) must succeed with Relay R offline.
	_ = relayHost.Close()

	got1, err := RequestBlock(ctx, hostB, hostA.ID(), WireBlockRequest{
		ContentID:  "track-direct-upgrade",
		BlockIndex: 1,
		BlockHash:  hash1,
	})
	if err != nil {
		t.Fatalf("expected Block 1 transfer to succeed over direct P2P with relay server shut down, got: %v", err)
	}
	if !bytes.Equal(got1, chunk1) {
		t.Fatalf("chunk 1 payload mismatch")
	}
}

func TestRelayLastResortFallbackWhenDirectBlocked(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. Relay R
	_, privR, _ := crypto.GenerateKeyPair()
	relayHost, err := NewHost(HostConfig{
		ListenAddr:         "/ip4/127.0.0.1/tcp/0",
		PrivateKey:         privR,
		EnableRelayService: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relayHost.Close()

	// 2. Seeder A
	_, privA, _ := crypto.GenerateKeyPair()
	hostA, err := NewHost(HostConfig{
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		PrivateKey: privA,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	chunk := bytes.Repeat([]byte("LAST-RESORT-RELAY-MELODY-"), 512)
	hash := crypto.HashChunkHex(chunk)
	NewProtocolHandler(hostA, &memoryProvider{
		blocks: map[string][]byte{hash: chunk},
	})

	// 3. Leecher B
	_, privB, _ := crypto.GenerateKeyPair()
	hostB, err := NewHost(HostConfig{
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
		PrivateKey: privB,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()

	_ = hostA.Connect(ctx, relayHost.AddrInfo())
	_, _ = hostA.ReserveRelay(ctx, relayHost.ID())
	_ = hostB.Connect(ctx, relayHost.AddrInfo())

	circuitAddr, _ := multiaddr.NewMultiaddr(fmt.Sprintf("/p2p/%s/p2p-circuit", relayHost.ID()))
	if err := hostB.Connect(ctx, peer.AddrInfo{
		ID:    hostA.ID(),
		Addrs: []multiaddr.Multiaddr{circuitAddr},
	}); err != nil {
		t.Fatalf("failed circuit connect: %v", err)
	}

	// Clear any direct loopback addresses of A from B's peerstore to simulate impenetrable double-random Symmetric NAT
	hostB.Raw().Peerstore().ClearAddrs(hostA.ID())
	hostB.Raw().Peerstore().AddAddr(hostA.ID(), circuitAddr, peerstore.PermanentAddrTTL)

	got, err := RequestBlock(ctx, hostB, hostA.ID(), WireBlockRequest{
		ContentID:  "track-relay-fallback",
		BlockIndex: 0,
		BlockHash:  hash,
	})
	if err != nil {
		t.Fatalf("expected last-resort relay fallback to succeed when direct hole punch is impossible, got: %v", err)
	}
	if !bytes.Equal(got, chunk) {
		t.Fatalf("payload mismatch on relay fallback")
	}
}
