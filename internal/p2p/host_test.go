package p2p

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"resonamesh/pkg/crypto"
)

func TestNewHostValidation(t *testing.T) {
	_, err := NewHost(HostConfig{
		ListenAddr: "/ip4/127.0.0.1/tcp/0",
	})
	if err == nil {
		t.Fatalf("expected error for missing private key, got nil")
	}

	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	_ = pub

	_, err = NewHost(HostConfig{
		PrivateKey: priv,
		ListenAddr: "",
	})
	if err == nil {
		t.Fatalf("expected error for empty listen addr, got nil")
	}
}

func TestHostLoopbackConnection(t *testing.T) {
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

	if len(hostA.Addrs()) == 0 || len(hostB.Addrs()) == 0 {
		t.Fatalf("expected non-empty listen addresses")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to host B: %v", err)
	}

	peersA := hostA.ConnectedPeers()
	if len(peersA) != 1 || peersA[0] != hostB.ID() {
		t.Fatalf("expected host A connected to host B %s, got %v", hostB.ID(), peersA)
	}

	peersB := hostB.ConnectedPeers()
	if len(peersB) != 1 || peersB[0] != hostA.ID() {
		t.Fatalf("expected host B connected to host A %s, got %v", hostA.ID(), peersB)
	}
}

func TestHostWebSocketConnection(t *testing.T) {
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	// Host B listens on WebSocket /ws multiaddr
	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0/ws",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to host B over WebSocket: %v", err)
	}

	peersA := hostA.ConnectedPeers()
	if len(peersA) != 1 || peersA[0] != hostB.ID() {
		t.Fatalf("expected host A connected to host B %s via WebSocket, got %v", hostB.ID(), peersA)
	}
}

func TestHostWebSocketWithHTTPHandler(t *testing.T) {
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("relay-ok"))
	})

	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0/ws",
		PrivateKey:   privB,
		DisableRelay: true,
		HTTPHandler:  httpMux,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostB.Close()

	// Extract the bound TCP port from hostB multiaddrs
	addrs := hostB.Addrs()
	if len(addrs) == 0 {
		t.Fatal("host B has no listen addrs")
	}
	var portStr string
	for _, a := range addrs {
		val, err := a.ValueForProtocol(multiaddr.P_TCP)
		if err == nil {
			portStr = val
			break
		}
	}
	if portStr == "" {
		t.Fatalf("could not extract TCP port from %v", addrs)
	}

	// 1. Verify HTTP request to the co-hosted HTTP handler
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/health", portStr))
	if err != nil {
		t.Fatalf("failed HTTP request to co-hosted endpoint: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "relay-ok" {
		t.Fatalf("expected 'relay-ok', got %q", string(body))
	}

	// 2. Verify LibP2P WebSocket client connection to the same port
	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to host B over WebSocket: %v", err)
	}

	peersB := hostB.ConnectedPeers()
	if len(peersB) != 1 || peersB[0] != hostA.ID() {
		t.Fatalf("expected host B connected to host A, got %v", peersB)
	}
}

func TestHostDualListenersWithHTTPHandler(t *testing.T) {
	_, privA, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	_, privB, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}

	httpMux := http.NewServeMux()
	httpMux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("dual-ok"))
	})

	// Host B listens on BOTH TCP /0 AND WebSocket /0/ws with HTTP handler
	hostB, err := NewHost(HostConfig{
		ListenAddrs:  []string{"/ip4/127.0.0.1/tcp/0", "/ip4/127.0.0.1/tcp/0/ws"},
		PrivateKey:   privB,
		DisableRelay: true,
		HTTPHandler:  httpMux,
	})
	if err != nil {
		t.Fatalf("failed to create host B with dual listeners: %v", err)
	}
	defer hostB.Close()

	// Locate WebSocket port
	var wsPortStr string
	for _, a := range hostB.Addrs() {
		// Check if it ends in /ws
		s := a.String()
		if len(s) > 3 && s[len(s)-3:] == "/ws" {
			val, err := a.ValueForProtocol(multiaddr.P_TCP)
			if err == nil {
				wsPortStr = val
				break
			}
		}
	}
	if wsPortStr == "" {
		t.Fatalf("could not extract WebSocket TCP port from addrs: %v", hostB.Addrs())
	}

	// 1. Check HTTP request to the WebSocket listener
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/status", wsPortStr))
	if err != nil {
		t.Fatalf("failed HTTP request to co-hosted endpoint: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "dual-ok" {
		t.Fatalf("expected 'dual-ok', got %q", string(body))
	}

	// 2. Connect client host A to host B
	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer hostA.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to host B with dual listeners: %v", err)
	}

	peersB := hostB.ConnectedPeers()
	if len(peersB) != 1 || peersB[0] != hostA.ID() {
		t.Fatalf("expected host B connected to host A, got %v", peersB)
	}
}



