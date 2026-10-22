package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"resonamesh/pkg/crypto"
)

func TestPeerTrackerOperations(t *testing.T) {
	tracker := NewPeerTracker()
	if tracker.Count() != 0 {
		t.Fatalf("expected 0 peers, got %d", tracker.Count())
	}

	ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	if err != nil {
		t.Fatalf("failed to create multiaddr: %v", err)
	}

	testPeerID, err := peer.Decode("12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP")
	if err != nil {
		t.Fatalf("failed to decode peer id: %v", err)
	}

	pi := peer.AddrInfo{
		ID:    testPeerID,
		Addrs: []multiaddr.Multiaddr{ma},
	}

	tracker.RecordPeer(pi, false)
	if tracker.Count() != 1 {
		t.Fatalf("expected 1 peer, got %d", tracker.Count())
	}

	peers := tracker.Peers()
	if len(peers) != 1 || peers[0].Connected {
		t.Fatalf("expected 1 disconnected peer, got %+v", peers)
	}

	tracker.SetConnected(testPeerID, true)
	peers = tracker.Peers()
	if !peers[0].Connected {
		t.Fatalf("expected peer to be connected")
	}

	// Update existing peer with new address
	ma2, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")
	tracker.RecordPeer(peer.AddrInfo{ID: testPeerID, Addrs: []multiaddr.Multiaddr{ma2}}, true)
	if tracker.Count() != 1 {
		t.Fatalf("expected count 1 after update, got %d", tracker.Count())
	}
	peers = tracker.Peers()
	if len(peers[0].Addrs) != 1 || peers[0].Addrs[0] != ma2.String() {
		t.Fatalf("expected updated address %s, got %v", ma2, peers[0].Addrs)
	}
}

func TestDiscoveryHandlePeerFoundAutoConnect(t *testing.T) {
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key A: %v", err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key B: %v", err)
	}

	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host A: %v", err)
	}
	defer hostA.Close()

	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host B: %v", err)
	}
	defer hostB.Close()

	tracker := NewPeerTracker()
	ds := NewDiscoveryService(hostA, tracker, "test-swarm")
	defer ds.Close()

	// Simulate self announcement - should be ignored
	ds.HandlePeerFound(hostA.AddrInfo())
	if tracker.Count() != 0 {
		t.Fatalf("expected self announcement to be ignored, got %d", tracker.Count())
	}

	// Announce remote host B
	ds.HandlePeerFound(hostB.AddrInfo())

	// Wait up to 3 seconds for background auto-connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	connected := false
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for auto-connection to host B")
		case <-ticker.C:
			peers := tracker.Peers()
			if len(peers) > 0 && peers[0].Connected {
				connected = true
				break
			}
		}
		if connected {
			break
		}
	}

	if len(hostA.ConnectedPeers()) != 1 || hostA.ConnectedPeers()[0] != hostB.ID() {
		t.Fatalf("expected host A connected to host B")
	}
}
