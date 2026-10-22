package p2p

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/moderation"
	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

func TestModerationProtocolExchange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Setup Moderator Node (Node B)
	modPub, modPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate moderator key: %v", err)
	}

	dirB := t.TempDir()
	dbB, err := storage.Open(filepath.Join(dirB, "mod.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db B: %v", err)
	}
	defer dbB.Close()

	engB, err := moderation.NewEngine(dbB, moderation.Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: crypto.PublicKeyToHex(modPub),
	}, modPriv)
	if err != nil {
		t.Fatalf("failed to create engine B: %v", err)
	}

	hostB, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   modPriv,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host B: %v", err)
	}
	defer hostB.Close()

	handlerB := NewModerationHandler(hostB, engB, modPriv)
	_ = handlerB

	// 2. Setup Client Node (Node A)
	_, userPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate user key: %v", err)
	}

	dirA := t.TempDir()
	dbA, err := storage.Open(filepath.Join(dirA, "user.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db A: %v", err)
	}
	defer dbA.Close()

	engA, err := moderation.NewEngine(dbA, moderation.Config{
		NetworkID:              "resonamesh-test",
		PolicyID:               "policy-test",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: crypto.PublicKeyToHex(modPub),
	}, userPriv)
	if err != nil {
		t.Fatalf("failed to create engine A: %v", err)
	}

	hostA, err := NewHost(HostConfig{
		ListenAddr:   "/ip4/127.0.0.1/tcp/0",
		PrivateKey:   userPriv,
		DisableRelay: true,
	})
	if err != nil {
		t.Fatalf("failed to create host A: %v", err)
	}
	defer hostA.Close()

	handlerA := NewModerationHandler(hostA, engA, nil)

	// Connect Node A to Node B
	if err := hostA.Connect(ctx, hostB.AddrInfo()); err != nil {
		t.Fatalf("failed to connect host A to B: %v", err)
	}

	// 3. User A submits a report into local outbox
	rep, err := engA.SubmitReport(ctx, moderation.CreateReportParams{
		ContentID:   "track-flagged-404",
		ReasonCode:  protocol.ReasonCopyrightClaim,
		Description: "Stolen track without license",
	})
	if err != nil {
		t.Fatalf("failed to submit report on node A: %v", err)
	}

	// 4. Send report over wire to moderator Node B
	receipt, err := handlerA.SendReport(ctx, hostB.ID(), rep)
	if err != nil {
		t.Fatalf("failed to send report from A to B: %v", err)
	}
	if receipt == nil {
		t.Fatal("expected receipt from moderator B, got nil")
	}

	// User A records the receipt
	if err := engA.RecordReceipt(ctx, receipt); err != nil {
		t.Fatalf("failed to record receipt on node A: %v", err)
	}

	// Verify report status is now ACCEPTED on node A
	reportsA, err := engA.ListReports(ctx)
	if err != nil || len(reportsA) != 1 || reportsA[0].Status != "ACCEPTED" {
		t.Fatalf("expected report status ACCEPTED on node A, got %+v", reportsA)
	}

	// 5. Moderator Node B issues a BLOCK decision
	dec, err := engB.CreateDecision(ctx, modPriv, moderation.CreateDecisionParams{
		ContentID:  "track-flagged-404",
		Action:     protocol.ActionBlock,
		ReasonCode: protocol.ReasonCopyrightClaim,
		ReasonText: "Confirmed copyright claim",
	})
	if err != nil {
		t.Fatalf("moderator failed to create decision on B: %v", err)
	}
	if dec.Sequence != 1 {
		t.Fatalf("expected decision sequence 1, got %d", dec.Sequence)
	}

	// 6. User A syncs decisions from Node B
	decisions, err := handlerA.SyncDecisions(ctx, hostB.ID(), 0)
	if err != nil {
		t.Fatalf("node A failed to sync decisions from B: %v", err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision from B, got %d", len(decisions))
	}

	// Apply received decision on node A
	if err := engA.ApplyDecision(ctx, decisions[0]); err != nil {
		t.Fatalf("node A failed to apply decision: %v", err)
	}

	// 7. Verify track is now blocked on node A!
	blocked, reason, err := engA.IsBlocked(ctx, "track-flagged-404", "")
	if err != nil || !blocked {
		t.Fatalf("expected track to be blocked on node A, got blocked=%v, reason=%s", blocked, reason)
	}
}
