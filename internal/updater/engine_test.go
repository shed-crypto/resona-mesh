package updater_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"resonamesh/internal/storage"
	"resonamesh/internal/updater"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestUpdaterLifecycleChunkingVerificationAndAtomicSwap(t *testing.T) {
	tmpDir := t.TempDir()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate release key: %v", err)
	}
	pubHex := crypto.PublicKeyToHex(pub)

	// 1. Setup shared BlockStore
	bs, err := storage.NewBlockStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create blockstore: %v", err)
	}

	// 2. Prepare synthetic source binary (600 KiB spanning 3 chunks)
	sourceBinaryData := make([]byte, 600*1024)
	for i := range sourceBinaryData {
		sourceBinaryData[i] = byte(i % 251)
	}
	sourceBinaryPath := filepath.Join(tmpDir, "simulated-v0.2.0.exe")
	if err := os.WriteFile(sourceBinaryPath, sourceBinaryData, 0755); err != nil {
		t.Fatalf("failed to write source binary: %v", err)
	}

	// 3. Prepare target executable (representing running resonamesh v0.1.0)
	initialBinaryData := []byte("INITIAL_RESONAMESH_V0.1.0_BINARY_PAYLOAD")
	targetExePath := filepath.Join(tmpDir, "current-resonamesh.exe")
	if err := os.WriteFile(targetExePath, initialBinaryData, 0755); err != nil {
		t.Fatalf("failed to write target executable: %v", err)
	}

	// 4. Seeder creates signed release manifest
	seederEngine, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.2.0",
		DataDir:        filepath.Join(tmpDir, "seeder-data"),
		BlockStore:     bs,
	})
	if err != nil {
		t.Fatalf("failed to create seeder engine: %v", err)
	}

	manifest, err := seederEngine.CreateRelease(sourceBinaryPath, "0.2.0", "Performance and P2P Swarm Enhancements", priv)
	if err != nil {
		t.Fatalf("CreateRelease failed: %v", err)
	}

	if len(manifest.BlockHashes) != 3 {
		t.Fatalf("expected 3 blocks for 600 KiB payload, got %d", len(manifest.BlockHashes))
	}

	// 5. Client node initializes with current version 0.1.0-alpha
	clientEngine, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.1.0-alpha",
		ExecutablePath: targetExePath,
		ReleasePubKey:  pubHex,
		DataDir:        filepath.Join(tmpDir, "client-data"),
		BlockStore:     bs,
	})
	if err != nil {
		t.Fatalf("failed to create client engine: %v", err)
	}

	// 6. Client receives and verifies release announcement
	accepted, err := clientEngine.ProcessAnnouncedRelease(manifest)
	if err != nil {
		t.Fatalf("ProcessAnnouncedRelease failed: %v", err)
	}
	if !accepted {
		t.Fatal("expected newer release to be accepted")
	}

	state := clientEngine.State()
	if state.Status != updater.StatusAvailable {
		t.Fatalf("expected status %s, got %s", updater.StatusAvailable, state.Status)
	}

	// 7. Client downloads and assembles binary
	ctx := context.Background()
	if err := clientEngine.DownloadRelease(ctx); err != nil {
		t.Fatalf("DownloadRelease failed: %v", err)
	}

	state = clientEngine.State()
	if state.Status != updater.StatusReady {
		t.Fatalf("expected status %s, got %s", updater.StatusReady, state.Status)
	}
	if state.ProgressPct != 100.0 {
		t.Fatalf("expected 100%% progress, got %f", state.ProgressPct)
	}

	// 8. Execute atomic update
	if err := clientEngine.ApplyUpdate(); err != nil {
		t.Fatalf("ApplyUpdate failed: %v", err)
	}

	// 9. Verify target executable now has the new 600 KiB binary data
	updatedData, err := os.ReadFile(targetExePath)
	if err != nil {
		t.Fatalf("failed to read updated target binary: %v", err)
	}
	if !bytes.Equal(updatedData, sourceBinaryData) {
		t.Fatal("updated executable does not match the source binary payload")
	}

	// 10. Verify backup exists
	backupData, err := os.ReadFile(targetExePath + ".old")
	if err != nil {
		t.Fatalf("failed to read backup executable: %v", err)
	}
	if !bytes.Equal(backupData, initialBinaryData) {
		t.Fatal("backup executable does not match the initial binary payload")
	}

	// 11. Test rollback
	if err := clientEngine.Rollback(); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}
	restoredData, err := os.ReadFile(targetExePath)
	if err != nil {
		t.Fatalf("failed to read restored binary: %v", err)
	}
	if !bytes.Equal(restoredData, initialBinaryData) {
		t.Fatal("restored executable does not match initial binary payload after rollback")
	}
}

func TestUpdaterSecurityRejections(t *testing.T) {
	tmpDir := t.TempDir()

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)

	engine, err := updater.NewEngine(updater.Config{
		CurrentVersion: "0.2.0",
		ReleasePubKey:  crypto.PublicKeyToHex(pub),
		DataDir:        tmpDir,
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	manifest := &protocol.ReleaseManifest{
		SchemaVersion:  protocol.SchemaReleaseV1,
		Version:        "0.3.0",
		TargetOS:       runtime.GOOS,
		TargetArch:     runtime.GOARCH,
		BinaryHash:     "1111222233334444555566667777888899990000111122223333444455556666",
		TotalSizeBytes: 1024,
		BlockSize:      protocol.BlockSize,
		BlockHashes:    []string{"1111222233334444555566667777888899990000111122223333444455556666"},
		ReleasePubKey:  crypto.PublicKeyToHex(otherPub), // untrusted key
		PublishedAt:    1000,
	}
	payload := protocol.CanonicalReleasePayload(manifest)
	sig := crypto.SignWithDomain(priv, crypto.DomainReleaseManifest, []byte(payload))
	manifest.Signature = crypto.PublicKeyToHex(sig) // hex signature

	// 1. Rejection of untrusted key
	if err := engine.VerifyManifest(manifest); err == nil {
		t.Fatal("expected untrusted release key error, got nil")
	}

	// 2. Rejection of mismatched OS
	mismatchedOSManifest := *manifest
	mismatchedOSManifest.ReleasePubKey = crypto.PublicKeyToHex(pub)
	mismatchedOSManifest.TargetOS = "unsupported_os"
	if err := engine.VerifyManifest(&mismatchedOSManifest); err == nil {
		t.Fatal("expected architecture mismatch error, got nil")
	}

	// 3. Rejection of older or same version
	sameVerManifest := *manifest
	sameVerManifest.ReleasePubKey = crypto.PublicKeyToHex(pub)
	sameVerManifest.Version = "0.1.0"
	canonical := protocol.CanonicalReleasePayload(&sameVerManifest)
	validSig := crypto.SignWithDomain(priv, crypto.DomainReleaseManifest, []byte(canonical))
	sameVerManifest.Signature = crypto.PublicKeyToHex(validSig)

	accepted, err := engine.ProcessAnnouncedRelease(&sameVerManifest)
	if accepted || err != updater.ErrVersionNotNewer {
		t.Fatalf("expected ErrVersionNotNewer, got accepted=%v, err=%v", accepted, err)
	}
}
