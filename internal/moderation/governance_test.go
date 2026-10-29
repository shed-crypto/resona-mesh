package moderation

import (
	"context"
	"encoding/hex"
	"testing"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestDelegatedModeratorLifecycle(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	rootPub, rootPriv, _ := crypto.GenerateKeyPair()
	rootPubHex := crypto.PublicKeyToHex(rootPub)

	modPub, modPriv, _ := crypto.GenerateKeyPair()
	modPubHex := crypto.PublicKeyToHex(modPub)

	cfg := Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: rootPubHex,
	}

	// 1. Node initialized with Root Admin as trusted authority
	engine, err := NewEngine(db, cfg, rootPriv)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	// Initially, modPub is NOT trusted
	if engine.isTrustedModerator(ctx, modPubHex) {
		t.Fatalf("modPub should not be trusted prior to delegation")
	}

	// 2. Root Admin delegates authority to modPub
	cert, err := engine.DelegateModerator(ctx, "APPROVE_MODERATOR", modPubHex, "JuniorMod", 3600)
	if err != nil {
		t.Fatalf("DelegateModerator failed: %v", err)
	}
	if cert.Action != "APPROVE_MODERATOR" || cert.ModeratorPubKey != modPubHex {
		t.Fatalf("unexpected certificate: %+v", cert)
	}

	// modPub should now be trusted
	if !engine.isTrustedModerator(ctx, modPubHex) {
		t.Fatalf("modPub should be trusted after delegation certificate")
	}

	// 3. Delegated moderator issues a BLOCK decision
	now := time.Now().Unix()
	decision := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       cfg.NetworkID,
		PolicyID:        cfg.PolicyID,
		PolicyEpoch:     1,
		ContentID:       "track-spam-123",
		Sequence:        1,
		Action:          "BLOCK",
		ReasonCode:      protocol.ReasonCopyrightClaim,
		ReasonText:      "Unauthorized reproduction",
		ModeratorKeyHex: modPubHex,
		IssuedAt:        now,
	}
	payload := protocol.CanonicalDecisionPayload(decision)
	sig := crypto.SignWithDomain(modPriv, crypto.DomainDecision, []byte(payload))
	decision.Signature = hex.EncodeToString(sig)

	// Peer applies decision signed by delegated moderator
	if err := engine.ApplyDecision(ctx, decision); err != nil {
		t.Fatalf("ApplyDecision signed by delegated moderator failed: %v", err)
	}

	// Verify track is blocked
	blocked, err := engine.ListBlocked(ctx)
	if err != nil || len(blocked) != 1 || blocked[0].ContentID != "track-spam-123" {
		t.Fatalf("expected track to be blocked: %+v", blocked)
	}

	time.Sleep(1100 * time.Millisecond)
	// 4. Root Admin revokes moderator
	revokeCert, err := engine.DelegateModerator(ctx, "REVOKE_MODERATOR", modPubHex, "JuniorMod", 0)
	if err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if revokeCert.Action != "REVOKE_MODERATOR" {
		t.Fatalf("expected REVOKE_MODERATOR")
	}

	// modPub should NO LONGER be trusted
	if engine.isTrustedModerator(ctx, modPubHex) {
		t.Fatalf("modPub should NOT be trusted after revocation")
	}

	// Subsequent decision from revoked moderator must be rejected
	decision2 := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       cfg.NetworkID,
		PolicyID:        cfg.PolicyID,
		PolicyEpoch:     1,
		ContentID:       "track-spam-456",
		Sequence:        2,
		Action:          "BLOCK",
		ReasonCode:      protocol.ReasonCopyrightClaim,
		ReasonText:      "Revoked moderator action",
		ModeratorKeyHex: modPubHex,
		IssuedAt:        now + 10,
	}
	payload2 := protocol.CanonicalDecisionPayload(decision2)
	sig2 := crypto.SignWithDomain(modPriv, crypto.DomainDecision, []byte(payload2))
	decision2.Signature = hex.EncodeToString(sig2)

	if err := engine.ApplyDecision(ctx, decision2); err != ErrUntrustedModerator {
		t.Fatalf("expected ErrUntrustedModerator for revoked moderator, got %v", err)
	}
}

func TestEmergencyKeyRotation(t *testing.T) {
	db, err := storage.Open(":memory:")
	if err != nil {
		t.Fatalf("storage.Open failed: %v", err)
	}
	defer db.Close()

	recPub, recPriv, _ := crypto.GenerateKeyPair()
	recPubHex := crypto.PublicKeyToHex(recPub)

	oldRootPub, _, _ := crypto.GenerateKeyPair()
	oldRootPubHex := crypto.PublicKeyToHex(oldRootPub)

	newRootPub, newRootPriv, _ := crypto.GenerateKeyPair()
	newRootPubHex := crypto.PublicKeyToHex(newRootPub)

	cfg := Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: oldRootPubHex,
		RecoveryPubKey:         recPubHex,
	}

	engine, err := NewEngine(db, cfg, nil)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()

	// Create and sign KeyRotationEvent
	rot := &protocol.KeyRotationEvent{
		NetworkID:      cfg.NetworkID,
		KeyRole:        "ROOT_ADMIN",
		NewPolicyEpoch: 2,
		OldPubKey:      oldRootPubHex,
		NewPubKey:      newRootPubHex,
		RecoveryPubKey: recPubHex,
		Reason:         "Primary admin key compromised",
		IssuedAt:       time.Now().Unix(),
	}
	rotPayload := protocol.CanonicalKeyRotationPayload(rot)
	sig := crypto.SignWithDomain(recPriv, crypto.DomainKeyRotation, []byte(rotPayload))
	rot.Signature = hex.EncodeToString(sig)
	rot.EventID = crypto.HashChunkHex([]byte(rotPayload))

	// Apply key rotation
	if err := engine.ApplyKeyRotation(ctx, rot); err != nil {
		t.Fatalf("ApplyKeyRotation failed: %v", err)
	}

	// Verify old root is no longer trusted, new root is trusted
	if engine.isTrustedModerator(ctx, oldRootPubHex) {
		t.Fatalf("old root key should no longer be trusted after rotation")
	}
	if !engine.isTrustedModerator(ctx, newRootPubHex) {
		t.Fatalf("new root key should be trusted after rotation")
	}

	// New root admin can sign decisions
	decision := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       cfg.NetworkID,
		PolicyID:        cfg.PolicyID,
		PolicyEpoch:     2,
		ContentID:       "track-post-rotation",
		Sequence:        1,
		Action:          "BLOCK",
		ReasonCode:      protocol.ReasonCopyrightClaim,
		ReasonText:      "Signed by new root admin",
		ModeratorKeyHex: newRootPubHex,
		IssuedAt:        time.Now().Unix(),
	}
	decPayload := protocol.CanonicalDecisionPayload(decision)
	decSig := crypto.SignWithDomain(newRootPriv, crypto.DomainDecision, []byte(decPayload))
	decision.Signature = hex.EncodeToString(decSig)

	if err := engine.ApplyDecision(ctx, decision); err != nil {
		t.Fatalf("decision from new root admin failed: %v", err)
	}

	// Re-applying rotation with lower or equal epoch must be rejected
	rotReplay := *rot
	rotReplay.NewPolicyEpoch = 2
	if err := engine.ApplyKeyRotation(ctx, &rotReplay); err == nil {
		t.Fatalf("expected error on replay rotation with same epoch, got nil")
	}

	// Rotation signed by wrong recovery key must be rejected
	_, fakeRecPriv, _ := crypto.GenerateKeyPair()
	rotFake := &protocol.KeyRotationEvent{
		NetworkID:      cfg.NetworkID,
		KeyRole:        "ROOT_ADMIN",
		NewPolicyEpoch: 3,
		OldPubKey:      newRootPubHex,
		NewPubKey:      oldRootPubHex,
		RecoveryPubKey: recPubHex,
		Reason:         "Unauthorized rollback",
		IssuedAt:       time.Now().Unix(),
	}
	fakePayload := protocol.CanonicalKeyRotationPayload(rotFake)
	fakeSig := crypto.SignWithDomain(fakeRecPriv, crypto.DomainKeyRotation, []byte(fakePayload))
	rotFake.Signature = hex.EncodeToString(fakeSig)

	if err := engine.ApplyKeyRotation(ctx, rotFake); err != ErrInvalidSignature {
		t.Fatalf("expected ErrInvalidSignature for forged recovery signature, got %v", err)
	}
}