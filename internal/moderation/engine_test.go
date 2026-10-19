package moderation

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)



func TestSubmitReport(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	_, nodePriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate node key: %v", err)
	}

	eng, err := NewEngine(db, Config{NetworkID: "test-net"}, nodePriv)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()

	// Missing content ID
	_, err = eng.SubmitReport(ctx, CreateReportParams{
		ReasonCode:  protocol.ReasonCopyrightClaim,
		Description: "test",
	})
	if err != ErrMissingContentID {
		t.Fatalf("expected ErrMissingContentID, got %v", err)
	}

	// Invalid reason code
	_, err = eng.SubmitReport(ctx, CreateReportParams{
		ContentID:   "content-1",
		ReasonCode:  "INVALID_CODE",
		Description: "test",
	})
	if err != ErrInvalidReasonCode {
		t.Fatalf("expected ErrInvalidReasonCode, got %v", err)
	}

	// Valid submission
	rep, err := eng.SubmitReport(ctx, CreateReportParams{
		ContentID:         "content-1",
		PayloadHash:       "hash-payload-1",
		ReasonCode:        protocol.ReasonCopyrightClaim,
		Description:       "Infringes copyright",
		EvidenceReference: "https://example.com/license",
	})
	if err != nil {
		t.Fatalf("failed to submit valid report: %v", err)
	}

	if rep.ContentID != "content-1" || rep.Signature == "" {
		t.Fatalf("invalid report fields: %+v", rep)
	}

	// Verify report is in outbox with PENDING status
	reports, err := eng.ListReports(ctx)
	if err != nil {
		t.Fatalf("failed to list reports: %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Status != "PENDING" || reports[0].ContentID != "content-1" {
		t.Fatalf("unexpected report state: %+v", reports[0])
	}
}

func TestReceiptFlow(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	modPub, modPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate moderator key: %v", err)
	}

	_, nodePriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate node key: %v", err)
	}

	eng, err := NewEngine(db, Config{
		NetworkID:              "test-net",
		TrustedModeratorPubKey: crypto.PublicKeyToHex(modPub),
	}, nodePriv)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()
	rep, err := eng.SubmitReport(ctx, CreateReportParams{
		ContentID:   "track-xyz",
		ReasonCode:  protocol.ReasonMisleadingMetadata,
		Description: "Wrong artist name",
	})
	if err != nil {
		t.Fatalf("failed to submit report: %v", err)
	}

	repID := protocol.ReportEventID(rep)

	// Moderator generates receipt
	rc, err := eng.CreateReceipt(ctx, modPriv, repID)
	if err != nil {
		t.Fatalf("failed to create receipt: %v", err)
	}

	// Apply receipt
	if err := eng.RecordReceipt(ctx, rc); err != nil {
		t.Fatalf("failed to record valid receipt: %v", err)
	}

	// Check report status is updated to ACCEPTED
	reports, err := eng.ListReports(ctx)
	if err != nil {
		t.Fatalf("failed to list reports: %v", err)
	}
	if len(reports) != 1 || reports[0].Status != "ACCEPTED" {
		t.Fatalf("expected report status ACCEPTED, got %+v", reports[0])
	}

	// Record receipt with forged signature should fail
	rcFake := &protocol.ReceiptEvent{
		ReportID:        repID,
		ModeratorKeyHex: crypto.PublicKeyToHex(modPub),
		ReceivedAt:      time.Now().Unix(),
		Signature:       "deadbeef",
	}
	if err := eng.RecordReceipt(ctx, rcFake); err == nil {
		t.Fatal("expected error for fake signature, got nil")
	}
}

func TestDecisionLifecycleAndDualMatching(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	modPub, modPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate moderator key: %v", err)
	}

	eng, err := NewEngine(db, Config{
		NetworkID:              "test-net",
		PolicyID:               "policy-v1",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: crypto.PublicKeyToHex(modPub),
	}, nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()

	// Initially not blocked
	blocked, _, err := eng.IsBlocked(ctx, "content-a", "hash-payload-a")
	if err != nil {
		t.Fatalf("IsBlocked error: %v", err)
	}
	if blocked {
		t.Fatal("expected track to not be blocked")
	}

	// Moderator issues BLOCK decision (seq 1)
	dec1, err := eng.CreateDecision(ctx, modPriv, CreateDecisionParams{
		ContentID:   "content-a",
		PayloadHash: "hash-payload-a",
		Action:      protocol.ActionBlock,
		ReasonCode:  protocol.ReasonCopyrightClaim,
		ReasonText:  "Takedown request approved",
	})
	if err != nil {
		t.Fatalf("failed to create decision 1: %v", err)
	}
	if dec1.Sequence != 1 {
		t.Fatalf("expected sequence 1, got %d", dec1.Sequence)
	}

	// Check dual matching: match by content_id
	blocked, reason, err := eng.IsBlocked(ctx, "content-a", "")
	if err != nil || !blocked {
		t.Fatalf("expected content-a to be blocked, got blocked=%v, reason=%s", blocked, reason)
	}

	// Check dual matching: match by payload_hash
	blocked, reason, err = eng.IsBlocked(ctx, "different-content-id", "hash-payload-a")
	if err != nil || !blocked {
		t.Fatalf("expected hash-payload-a to be blocked, got blocked=%v, reason=%s", blocked, reason)
	}

	// Moderator issues RESTORE decision (seq 2)
	dec2, err := eng.CreateDecision(ctx, modPriv, CreateDecisionParams{
		ContentID:   "content-a",
		PayloadHash: "hash-payload-a",
		Action:      protocol.ActionRestore,
		ReasonCode:  protocol.ReasonOther,
		ReasonText:  "DMCA counter-notice accepted",
	})
	if err != nil {
		t.Fatalf("failed to create decision 2: %v", err)
	}
	if dec2.Sequence != 2 {
		t.Fatalf("expected sequence 2, got %d", dec2.Sequence)
	}

	// Now neither content_id nor payload_hash should be blocked
	blocked, _, err = eng.IsBlocked(ctx, "content-a", "hash-payload-a")
	if err != nil || blocked {
		t.Fatalf("expected unblocked after restore, got blocked=%v", blocked)
	}
}

func TestForkAndGapDetection(t *testing.T) {
	dir := t.TempDir()
	db, err := storage.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	modPub, modPriv, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate moderator key: %v", err)
	}

	eng, err := NewEngine(db, Config{
		NetworkID:              "test-net",
		PolicyID:               "policy-v1",
		PolicyEpoch:            1,
		TrustedModeratorPubKey: crypto.PublicKeyToHex(modPub),
	}, nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	ctx := context.Background()

	// Seq 1 decision
	_, err = eng.CreateDecision(ctx, modPriv, CreateDecisionParams{
		ContentID:   "content-fork",
		Action:      protocol.ActionBlock,
		ReasonCode:  protocol.ReasonCopyrightClaim,
		ReasonText:  "Initial block",
	})
	if err != nil {
		t.Fatalf("failed to create decision 1: %v", err)
	}

	// Attempt to create a decision with sequence gap (seq 3 instead of 2)
	gapDecision := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       "test-net",
		PolicyID:        "policy-v1",
		PolicyEpoch:     1,
		ContentID:       "content-fork",
		Sequence:        3,
		PreviousEventID: "some-unknown-id",
		Action:          protocol.ActionBlock,
		ReasonCode:      protocol.ReasonCopyrightClaim,
		ReasonText:      "Gap block",
		ModeratorKeyHex: crypto.PublicKeyToHex(modPub),
		IssuedAt:        time.Now().Unix(),
	}
	canon := protocol.CanonicalDecisionPayload(gapDecision)
	sig := crypto.SignWithDomain(modPriv, crypto.DomainDecision, []byte(canon))
	gapDecision.Signature = crypto.PublicKeyToHex(sig)

	err = eng.ApplyDecision(ctx, gapDecision)
	if err != ErrSequenceGap {
		t.Fatalf("expected ErrSequenceGap, got %v", err)
	}

	// Attempt a fork with same sequence 1 but different action
	forkDecision := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       "test-net",
		PolicyID:        "policy-v1",
		PolicyEpoch:     1,
		ContentID:       "content-fork",
		Sequence:        1,
		PreviousEventID: "",
		Action:          protocol.ActionRestore,
		ReasonCode:      protocol.ReasonOther,
		ReasonText:      "Fork restore",
		ModeratorKeyHex: crypto.PublicKeyToHex(modPub),
		IssuedAt:        time.Now().Unix(),
	}
	canonFork := protocol.CanonicalDecisionPayload(forkDecision)
	sigFork := crypto.SignWithDomain(modPriv, crypto.DomainDecision, []byte(canonFork))
	forkDecision.Signature = crypto.PublicKeyToHex(sigFork)

	err = eng.ApplyDecision(ctx, forkDecision)
	if err != ErrForkDetected {
		t.Fatalf("expected ErrForkDetected, got %v", err)
	}
}
