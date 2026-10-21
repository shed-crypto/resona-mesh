package p2p

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"resonamesh/internal/importer"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

type memoryCatalog struct {
	manifests map[string]*protocol.Manifest
}

func (m *memoryCatalog) ListManifests(_ context.Context) ([]*protocol.Manifest, error) {
	result := make([]*protocol.Manifest, 0, len(m.manifests))
	for _, man := range m.manifests {
		result = append(result, man)
	}
	return result, nil
}

func (m *memoryCatalog) GetManifest(_ context.Context, contentID string) (*protocol.Manifest, error) {
	man, ok := m.manifests[contentID]
	if !ok {
		return nil, errorsNewNotFound()
	}
	return man, nil
}

func (m *memoryCatalog) RegisterRemoteManifest(_ context.Context, man *protocol.Manifest) error {
	m.manifests[man.ContentID] = man
	return nil
}

func errorsNewNotFound() error {
	return ErrBlockNotFound
}

func TestVerifyManifestValidation(t *testing.T) {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	blockHashes := []string{
		"1111111111111111111111111111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222222222222222222222222222",
	}
	contentID := crypto.HashContent(blockHashes)

	m := &protocol.Manifest{
		SchemaVersion:   "1.0.0",
		ContentID:       contentID,
		Title:           "Decentralized Track",
		Artist:          "P2P Artist",
		DurationMs:      180000,
		BitrateKbps:     320,
		TotalSizeBytes:  512 * 1024,
		BlockSize:       protocol.BlockSize,
		BlockHashes:     blockHashes,
		PublisherKeyHex: hex.EncodeToString(pub),
		PublishedAt:     time.Now().Unix(),
		LicenseCode:     protocol.LicenseCCBY40,
	}

	payload := importer.CanonicalManifestPayload(m)
	sig := crypto.SignWithDomain(priv, crypto.DomainManifest, []byte(payload))
	m.Signature = hex.EncodeToString(sig)

	// 1. Valid manifest
	if err := VerifyManifest(m); err != nil {
		t.Fatalf("expected valid manifest to pass, got: %v", err)
	}

	// 2. Tampered content ID
	tampered := *m
	tampered.ContentID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	if err := VerifyManifest(&tampered); err == nil {
		t.Fatalf("expected tampered content id to fail")
	}

	// 3. Corrupted signature
	badSig := *m
	badSigBytes := make([]byte, len(sig))
	copy(badSigBytes, sig)
	badSigBytes[0] ^= 0xFF
	badSig.Signature = hex.EncodeToString(badSigBytes)
	if err := VerifyManifest(&badSig); err == nil {
		t.Fatalf("expected corrupted signature to fail")
	}
}

func TestCatalogSyncerEndToEnd(t *testing.T) {
	pubA, privA, err := crypto.GenerateKeyPair()
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

	// Prepare signed manifest in Host A's catalog
	blockHashes := []string{
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	contentID := crypto.HashContent(blockHashes)

	m := &protocol.Manifest{
		SchemaVersion:   "1.0.0",
		ContentID:       contentID,
		Title:           "Swarm Anthem",
		Artist:          "Resona Mesh",
		DurationMs:      210000,
		BitrateKbps:     320,
		TotalSizeBytes:  512 * 1024,
		BlockSize:       protocol.BlockSize,
		BlockHashes:     blockHashes,
		PublisherKeyHex: hex.EncodeToString(pubA),
		PublishedAt:     time.Now().Unix(),
		LicenseCode:     protocol.LicenseCCBY40,
	}
	payload := importer.CanonicalManifestPayload(m)
	sig := crypto.SignWithDomain(privA, crypto.DomainManifest, []byte(payload))
	m.Signature = hex.EncodeToString(sig)

	catA := &memoryCatalog{
		manifests: map[string]*protocol.Manifest{
			contentID: m,
		},
	}
	catB := &memoryCatalog{
		manifests: make(map[string]*protocol.Manifest),
	}

	// Register syncer on Host A
	_, err = NewCatalogSyncer(hostA, catA)
	if err != nil {
		t.Fatalf("failed to create syncer on A: %v", err)
	}

	// Register syncer on Host B
	syncerB, err := NewCatalogSyncer(hostB, catB)
	if err != nil {
		t.Fatalf("failed to create syncer on B: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := hostB.Connect(ctx, hostA.AddrInfo()); err != nil {
		t.Fatalf("failed to connect B to A: %v", err)
	}

	// Host B syncs from Host A
	imported, err := syncerB.SyncFromPeer(ctx, hostA.ID())
	if err != nil {
		t.Fatalf("failed to sync manifests from peer: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 imported manifest, got %d", imported)
	}

	// Verify Host B now has the verified manifest
	syncedManifest, ok := catB.manifests[contentID]
	if !ok {
		t.Fatalf("manifest not found in Host B catalog")
	}
	if syncedManifest.Title != "Swarm Anthem" || syncedManifest.Artist != "Resona Mesh" {
		t.Fatalf("synced metadata mismatch: %+v", syncedManifest)
	}
}
