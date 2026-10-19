package protocol

import (
	"testing"
)

func TestCanonicalReportPayloadAndID(t *testing.T) {
	report := &ReportEvent{
		SchemaVersion:     "1.0.0",
		NetworkID:         "resonamesh-main",
		PolicyID:          "policy-default",
		ContentID:         "test-content-123",
		PayloadHash:       "hash-payload-abc",
		ReporterKeyHex:    "abcd1234abcd1234",
		ReasonCode:        ReasonCopyrightClaim,
		Description:       "Infringing track",
		EvidenceReference: "https://example.com/proof",
		CreatedAt:         1700000000,
		Nonce:             "random-nonce-999",
	}

	payload := CanonicalReportPayload(report)
	if payload == "" {
		t.Fatal("expected non-empty canonical report payload")
	}

	id1 := ReportEventID(report)
	id2 := ReportEventID(report)
	if id1 == "" || id1 != id2 {
		t.Fatalf("expected deterministic report event ID, got %s and %s", id1, id2)
	}
}

func TestCanonicalDecisionPayloadAndID(t *testing.T) {
	decision := &DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       "resonamesh-main",
		PolicyID:        "policy-default",
		PolicyEpoch:     1,
		ContentID:       "test-content-123",
		PayloadHash:     "hash-payload-abc",
		Sequence:        1,
		PreviousEventID: "",
		Action:          ActionBlock,
		ReasonCode:      ReasonCopyrightClaim,
		ReasonText:      "Verified DMCA claim",
		ModeratorKeyHex: "modkey1234",
		IssuedAt:        1700000100,
	}

	payload := CanonicalDecisionPayload(decision)
	if payload == "" {
		t.Fatal("expected non-empty canonical decision payload")
	}

	id1 := DecisionEventID(decision)
	id2 := DecisionEventID(decision)
	if id1 == "" || id1 != id2 {
		t.Fatalf("expected deterministic decision event ID, got %s and %s", id1, id2)
	}
}

func TestCanonicalReceiptPayload(t *testing.T) {
	receipt := &ReceiptEvent{
		ReportID:        "rep-id-123",
		ModeratorKeyHex: "modkey1234",
		ReceivedAt:      1700000050,
	}

	payload := CanonicalReceiptPayload(receipt)
	expected := "rep-id-123|modkey1234|1700000050"
	if payload != expected {
		t.Fatalf("expected payload %q, got %q", expected, payload)
	}
}
