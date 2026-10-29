package protocol_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestReleaseManifestSigningAndVerification(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	pubHex := crypto.PublicKeyToHex(pub)

	manifest := &protocol.ReleaseManifest{
		SchemaVersion:  protocol.SchemaReleaseV1,
		Version:        "0.2.0",
		TargetOS:       "windows",
		TargetArch:     "amd64",
		BinaryHash:     "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		TotalSizeBytes: 1048576,
		BlockSize:      protocol.BlockSize,
		BlockHashes: []string{
			"aaaa111122223333444455556666777788889999aaaabbbbccccddddeeeeffff",
			"bbbb111122223333444455556666777788889999aaaabbbbccccddddeeeeffff",
		},
		ReleasePubKey: pubHex,
		PublishedAt:   time.Now().Unix(),
		ReleaseNotes:  "ResonaMesh v0.2.0: Swarm performance upgrades",
	}

	payload := protocol.CanonicalReleasePayload(manifest)
	if !strings.Contains(payload, manifest.Version) || !strings.Contains(payload, manifest.BinaryHash) {
		t.Fatalf("canonical payload missing essential fields: %s", payload)
	}

	// Sign with DomainReleaseManifest
	sig := crypto.SignWithDomain(priv, crypto.DomainReleaseManifest, []byte(payload))
	manifest.Signature = hex.EncodeToString(sig)

	eventID := protocol.ReleaseManifestID(manifest)
	if len(eventID) != 64 {
		t.Fatalf("expected 64-char hex event ID, got: %s", eventID)
	}

	// Verify valid signature
	sigBytes, err := hex.DecodeString(manifest.Signature)
	if err != nil {
		t.Fatalf("failed to decode signature hex: %v", err)
	}
	if !crypto.VerifyWithDomain(pub, crypto.DomainReleaseManifest, []byte(payload), sigBytes) {
		t.Fatal("valid release manifest signature failed verification")
	}

	// Verify rejection under wrong domain
	if crypto.VerifyWithDomain(pub, crypto.DomainManifest, []byte(payload), sigBytes) {
		t.Fatal("expected signature verification failure under mismatched domain")
	}

	// Verify rejection under tampered binary hash
	tamperedManifest := *manifest
	tamperedManifest.BinaryHash = "0000000000000000000000000000000000000000000000000000000000000000"
	tamperedPayload := protocol.CanonicalReleasePayload(&tamperedManifest)
	if crypto.VerifyWithDomain(pub, crypto.DomainReleaseManifest, []byte(tamperedPayload), sigBytes) {
		t.Fatal("expected signature verification failure for tampered payload")
	}
}
