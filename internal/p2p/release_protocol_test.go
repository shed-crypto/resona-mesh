package p2p_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"runtime"
	"testing"
	"time"

	"resonamesh/internal/p2p"
	"resonamesh/internal/updater"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestP2PReleaseAnnouncementAndQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpA := t.TempDir()
	tmpB := t.TempDir()

	// 1. Release authority signing key
	relPub, relPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate release key: %v", err)
	}
	relPubHex := crypto.PublicKeyToHex(relPub)

	// 2. Host keys
	_, privA, _ := crypto.GenerateKeyPair()
	_, privB, _ := crypto.GenerateKeyPair()

	hostA, err := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privA,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host A: %v", err)
	}
	defer hostA.Close()

	hostB, err := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privB,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host B: %v", err)
	}
	defer hostB.Close()

	// 3. Connect hosts
	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to host B: %v", err)
	}

	// 4. Node A updater (already on 0.2.0, holds latest release)
	updaterA, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.2.0",
		ReleasePubKey:  relPubHex,
		DataDir:        tmpA,
	})
	if err != nil {
		t.Fatalf("failed to create updater A: %v", err)
	}

	// 5. Node B updater (on 0.1.0-alpha, needs update)
	updaterB, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.1.0-alpha",
		ReleasePubKey:  relPubHex,
		DataDir:        tmpB,
	})
	if err != nil {
		t.Fatalf("failed to create updater B: %v", err)
	}

	handlerA := p2p.NewReleaseHandler(hostA, updaterA)
	handlerB := p2p.NewReleaseHandler(hostB, updaterB)

	// 6. Create valid release manifest for v0.2.0
	manifest := &protocol.ReleaseManifest{
		SchemaVersion:  protocol.SchemaReleaseV1,
		Version:        "0.2.0",
		TargetOS:       runtime.GOOS,
		TargetArch:     runtime.GOARCH,
		BinaryHash:     "11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff",
		TotalSizeBytes: 2048,
		BlockSize:      protocol.BlockSize,
		BlockHashes:    []string{"11223344556677889900aabbccddeeff11223344556677889900aabbccddeeff"},
		ReleasePubKey:  relPubHex,
		PublishedAt:    time.Now().Unix(),
		ReleaseNotes:   "P2P Wire Protocol Test Release",
	}
	payload := protocol.CanonicalReleasePayload(manifest)
	sig := crypto.SignWithDomain(relPriv, crypto.DomainReleaseManifest, []byte(payload))
	manifest.Signature = hex.EncodeToString(sig)

	// 7. Announce from Node A to Node B
	if err := handlerA.AnnounceRelease(ctx, hostB.ID(), manifest); err != nil {
		t.Fatalf("AnnounceRelease failed: %v", err)
	}

	// 8. Verify Node B accepted the release
	stateB := updaterB.State()
	if stateB.Status != updater.StatusAvailable {
		t.Fatalf("expected node B status %s, got %s", updater.StatusAvailable, stateB.Status)
	}
	if stateB.AvailableRelease == nil || stateB.AvailableRelease.Version != "0.2.0" {
		t.Fatalf("node B did not store available release v0.2.0")
	}

	// 9. Query from Node A to Node B (Node B now has the release available)
	queriedManifest, err := handlerA.QueryPeer(ctx, hostB.ID())
	if err != nil {
		t.Fatalf("QueryPeer failed: %v", err)
	}
	if queriedManifest == nil || queriedManifest.Version != "0.2.0" {
		t.Fatalf("expected queried manifest to be v0.2.0, got: %v", queriedManifest)
	}

	// 10. Broadcast test
	sentCount, err := handlerA.BroadcastRelease(ctx, manifest)
	if err != nil {
		t.Fatalf("BroadcastRelease error: %v", err)
	}
	if sentCount != 1 {
		t.Fatalf("expected broadcast to 1 peer, got: %d", sentCount)
	}
	_ = handlerB
}

func TestReleaseHandlerReactivePeerHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tmpC := t.TempDir()
	tmpD := t.TempDir()

	relPub, relPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	relPubHex := crypto.PublicKeyToHex(relPub)

	_, privC, _ := crypto.GenerateKeyPair()
	_, privD, _ := crypto.GenerateKeyPair()

	hostC, err := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privC,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host C: %v", err)
	}
	defer hostC.Close()

	hostD, err := p2p.NewHost(p2p.HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   privD,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host D: %v", err)
	}
	defer hostD.Close()

	// Node C is on 0.1.0-alpha, status Idle
	updaterC, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.1.0-alpha",
		ReleasePubKey:  relPubHex,
		DataDir:        tmpC,
	})
	if err != nil {
		t.Fatalf("failed to create updater C: %v", err)
	}

	// Node D is currently on 0.1.0-alpha, staging v0.2.0 release
	updaterD, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.1.0-alpha",
		ReleasePubKey:  relPubHex,
		DataDir:        tmpD,
	})
	if err != nil {
		t.Fatalf("failed to create updater D: %v", err)
	}

	manifest := &protocol.ReleaseManifest{
		SchemaVersion:  protocol.SchemaReleaseV1,
		Version:        "0.2.0",
		TargetOS:       runtime.GOOS,
		TargetArch:     runtime.GOARCH,
		BinaryHash:     "aabbccddeeff11223344556677889900aabbccddeeff11223344556677889900",
		TotalSizeBytes: 4096,
		BlockSize:      protocol.BlockSize,
		BlockHashes:    []string{"aabbccddeeff11223344556677889900aabbccddeeff11223344556677889900"},
		ReleasePubKey:  relPubHex,
		PublishedAt:    time.Now().Unix(),
		ReleaseNotes:   "Reactive Handshake Verification",
	}
	payload := protocol.CanonicalReleasePayload(manifest)
	sig := crypto.SignWithDomain(relPriv, crypto.DomainReleaseManifest, []byte(payload))
	manifest.Signature = hex.EncodeToString(sig)

	// Register release on updaterD
	_, err = updaterD.ProcessAnnouncedRelease(manifest)
	if err != nil {
		t.Fatalf("failed to stage release on updater D: %v", err)
	}

	// Initialize handlers with reactive NotifyBundle listeners
	_ = p2p.NewReleaseHandler(hostC, updaterC)
	_ = p2p.NewReleaseHandler(hostD, updaterD)

	// Connect hosts. This triggers ConnectedF -> onPeerConnected asynchronously
	if err := hostC.Connect(ctx, hostD.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host C to host D: %v", err)
	}

	// Wait up to 3 seconds for the reactive handshake to exchange manifests
	deadline := time.Now().Add(4 * time.Second)
	updated := false
	for time.Now().Before(deadline) {
		if updaterC.State().Status == updater.StatusAvailable {
			updated = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !updated {
		t.Fatalf("Node C did not reactively receive the release announcement on peer connect; status=%s", updaterC.State().Status)
	}
	if updaterC.State().AvailableRelease == nil || updaterC.State().AvailableRelease.Version != "0.2.0" {
		t.Fatalf("Node C reactive release version mismatch, got: %v", updaterC.State().AvailableRelease)
	}
}

