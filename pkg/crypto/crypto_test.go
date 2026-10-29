package crypto_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"resonamesh/pkg/crypto"
)

func TestKeyPairGeneration(t *testing.T) {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		t.Fatalf("expected public key size %d, got %d", ed25519.PublicKeySize, len(pub))
	}
	if len(priv) != ed25519.PrivateKeySize {
		t.Fatalf("expected private key size %d, got %d", ed25519.PrivateKeySize, len(priv))
	}
}

func TestDomainSeparatedSigning(t *testing.T) {
	pub, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	payload := []byte("manifest-payload-content-hash")
	sig := crypto.SignWithDomain(priv, crypto.DomainManifest, payload)

	if !crypto.VerifyWithDomain(pub, crypto.DomainManifest, payload, sig) {
		t.Fatal("expected valid signature verification for correct domain")
	}

	// Tampered payload
	tampered := []byte("manifest-payload-content-hash-tampered")
	if crypto.VerifyWithDomain(pub, crypto.DomainManifest, tampered, sig) {
		t.Fatal("expected signature verification failure for tampered payload")
	}

	// Domain mismatch rejection
	if crypto.VerifyWithDomain(pub, crypto.DomainDecision, payload, sig) {
		t.Fatal("expected signature verification failure for mismatched domain")
	}

	// Corrupted signature
	corruptedSig := append([]byte(nil), sig...)
	corruptedSig[0] ^= 0xFF
	if crypto.VerifyWithDomain(pub, crypto.DomainManifest, payload, corruptedSig) {
		t.Fatal("expected signature verification failure for corrupted signature bytes")
	}
}

func TestHexKeyEncoding(t *testing.T) {
	pub, _, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	hexPub := crypto.PublicKeyToHex(pub)
	decoded, err := crypto.HexToPublicKey(hexPub)
	if err != nil {
		t.Fatalf("failed to decode valid hex public key: %v", err)
	}

	if !bytes.Equal(pub, decoded) {
		t.Fatal("decoded public key does not match original bytes")
	}

	// Invalid hex test
	_, err = crypto.HexToPublicKey("invalid-hex-string")
	if err == nil {
		t.Fatal("expected error for invalid hex string")
	}

	// Invalid length test
	shortHex := hex.EncodeToString([]byte("short-key"))
	_, err = crypto.HexToPublicKey(shortHex)
	if err == nil {
		t.Fatal("expected error for incorrect key length")
	}
}

func TestChunkHashing(t *testing.T) {
	// 256 KiB zero-buffer
	chunk := make([]byte, 262144)
	hexHash := crypto.HashChunkHex(chunk)

	if len(hexHash) != 64 {
		t.Fatalf("expected 64-char hex hash, got length %d", len(hexHash))
	}

	if !crypto.VerifyChunk(chunk, hexHash) {
		t.Fatal("expected chunk verification to succeed for matching hash")
	}

	chunk[0] = 0x01
	if crypto.VerifyChunk(chunk, hexHash) {
		t.Fatal("expected chunk verification to fail after mutating byte")
	}
}

func TestHashContent(t *testing.T) {
	hashes := []string{
		"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb",
	}

	contentID1 := crypto.HashContent(hashes)
	contentID2 := crypto.HashContent(hashes)

	if contentID1 != contentID2 {
		t.Fatal("expected HashContent to be strictly deterministic")
	}

	if len(contentID1) != 64 {
		t.Fatalf("expected 64-character SHA-256 hex string, got %d", len(contentID1))
	}
}

func TestPrivateKeyHexEncoding(t *testing.T) {
	_, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	hexPriv := crypto.PrivateKeyToHex(priv)
	decoded, err := crypto.HexToPrivateKey(hexPriv)
	if err != nil {
		t.Fatalf("failed to decode valid hex private key: %v", err)
	}

	if !bytes.Equal(priv, decoded) {
		t.Fatal("decoded private key does not match original bytes")
	}

	// Invalid hex test
	_, err = crypto.HexToPrivateKey("invalid-hex-string")
	if err == nil {
		t.Fatal("expected error for invalid hex string")
	}

	// Invalid length test
	shortHex := hex.EncodeToString([]byte("short-key"))
	_, err = crypto.HexToPrivateKey(shortHex)
	if err == nil {
		t.Fatal("expected error for incorrect key length")
	}
}

func TestLoadOrCreatePrivateKey(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "sub", "test_node.key")

	// First call: should generate and save key
	priv1, err := crypto.LoadOrCreatePrivateKey(keyPath)
	if err != nil {
		t.Fatalf("failed to create new private key: %v", err)
	}
	if len(priv1) != ed25519.PrivateKeySize {
		t.Fatalf("expected key length %d, got %d", ed25519.PrivateKeySize, len(priv1))
	}

	// Second call: should load existing key
	priv2, err := crypto.LoadOrCreatePrivateKey(keyPath)
	if err != nil {
		t.Fatalf("failed to load existing private key: %v", err)
	}

	if !bytes.Equal(priv1, priv2) {
		t.Fatal("loaded private key does not match previously generated key")
	}

	// Test corrupted key file
	corruptPath := filepath.Join(tempDir, "corrupt.key")
	if err := os.WriteFile(corruptPath, []byte("bad-hex-data"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = crypto.LoadOrCreatePrivateKey(corruptPath)
	if err == nil {
		t.Fatal("expected error when loading corrupted key file")
	}
}

func TestSavePrivateKey(t *testing.T) {
	tempDir := t.TempDir()
	keyPath := filepath.Join(tempDir, "sub", "saved_node.key")

	_, priv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	if err := crypto.SavePrivateKey(keyPath, priv); err != nil {
		t.Fatalf("failed to save private key: %v", err)
	}

	loaded, err := crypto.LoadOrCreatePrivateKey(keyPath)
	if err != nil {
		t.Fatalf("failed to load saved private key: %v", err)
	}

	if !bytes.Equal(priv, loaded) {
		t.Fatal("loaded key does not match saved key")
	}
}

