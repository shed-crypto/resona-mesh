package p2p

import (
	"context"
	"testing"
	"time"

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
