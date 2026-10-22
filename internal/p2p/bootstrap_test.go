package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"resonamesh/pkg/crypto"
)

func TestSeedResolverDirectMultiaddrs(t *testing.T) {
	resolver := NewSeedResolver(nil)
	validAddr := "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP"
	malformedAddr := "not-a-valid-multiaddress"

	infos, err := resolver.ResolveSeeds(context.Background(), []string{validAddr, malformedAddr, ""})
	if err != nil {
		t.Fatalf("unexpected error resolving seeds: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 resolved peer info, got %d", len(infos))
	}
	if infos[0].ID.String() != "12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP" {
		t.Errorf("expected peer ID 12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP, got %s", infos[0].ID.String())
	}
}

func TestSeedResolverRemoteHTTP(t *testing.T) {
	validAddr := "/ip4/192.168.1.50/tcp/4001/p2p/12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{validAddr})
	}))
	defer ts.Close()

	resolver := NewSeedResolver(ts.Client())
	infos, err := resolver.ResolveSeeds(context.Background(), []string{ts.URL})
	if err != nil {
		t.Fatalf("unexpected error resolving remote seeds: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 resolved remote peer, got %d", len(infos))
	}
	if infos[0].ID.String() != "12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP" {
		t.Errorf("unexpected peer ID: %s", infos[0].ID.String())
	}
}

func TestSeedResolverRemoteHTTPFailureTolerance(t *testing.T) {
	// 1. Failing server (500)
	tsFail := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer tsFail.Close()

	// 2. Working direct address
	validAddr := "/ip4/127.0.0.1/tcp/4001/p2p/12D3KooWDDkhy8WoLoni7vDgFYGZfkQeuXRCDgNTCpD8U6N5PSLP"

	resolver := NewSeedResolver(nil)
	infos, err := resolver.ResolveSeeds(context.Background(), []string{tsFail.URL, validAddr})
	if err != nil {
		t.Fatalf("resolver should tolerate remote failure without halting, got: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("expected 1 resolved peer from fallback, got %d", len(infos))
	}
}

func TestBootstrapSwarmConnection(t *testing.T) {
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

	// Mock HTTP server delivering host B's address
	hostBAddr := fmt.Sprintf("%s/p2p/%s", hostB.Addrs()[0].String(), hostB.ID().String())
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]string{hostBAddr})
	}))
	defer ts.Close()

	tracker := NewPeerTracker()
	var connectedPeer string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := BootstrapSwarm(ctx, hostA, tracker, []string{ts.URL}, func(p peer.ID) {
		connectedPeer = p.String()
	})
	if err != nil {
		t.Fatalf("bootstrap swarm failed: %v", err)
	}

	if count != 1 {
		t.Fatalf("expected 1 connected peer, got %d", count)
	}

	// Verify host A is connected to host B
	peersA := hostA.ConnectedPeers()
	if len(peersA) != 1 || peersA[0] != hostB.ID() {
		t.Fatalf("expected host A to be connected to host B, got: %v", peersA)
	}

	if tracker.Count() != 1 {
		t.Fatalf("expected 1 peer in tracker, got %d", tracker.Count())
	}
	if !tracker.Peers()[0].Connected {
		t.Errorf("expected peer record to be marked connected")
	}
	if connectedPeer != hostB.ID().String() {
		t.Errorf("expected callback with host B peer ID, got %s", connectedPeer)
	}
}

