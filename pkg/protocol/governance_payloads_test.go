package protocol

import (
	"testing"
)

func TestCanonicalModeratorCertPayload(t *testing.T) {
	cert := &ModeratorCertificate{
		CertID:          "cert-1",
		NetworkID:       "resonamesh-main",
		Action:          "APPROVE_MODERATOR",
		ModeratorPubKey: "ABCDEF1234567890",
		Alias:           "LeadMod",
		IssuedAt:        1700000000,
		ExpiresAt:       1800000000,
		RootAdminPubKey: "1234567890ABCDEF",
		Signature:       "sig",
	}

	payload := CanonicalModeratorCertPayload(cert)
	expected := "resonamesh-main|APPROVE_MODERATOR|abcdef1234567890|LeadMod|1700000000|1800000000|1234567890abcdef"
	if payload != expected {
		t.Fatalf("expected payload %q, got %q", expected, payload)
	}
}

func TestCanonicalKeyRotationPayload(t *testing.T) {
	rot := &KeyRotationEvent{
		EventID:        "rot-1",
		NetworkID:      "resonamesh-main",
		KeyRole:        "ROOT_ADMIN",
		NewPolicyEpoch: 2,
		OldPubKey:      "AAAA1111",
		NewPubKey:      "BBBB2222",
		RecoveryPubKey: "CCCC3333",
		Reason:         "Periodic key rotation",
		IssuedAt:       1700000000,
		Signature:      "sig",
	}

	payload := CanonicalKeyRotationPayload(rot)
	expected := "resonamesh-main|ROOT_ADMIN|2|aaaa1111|bbbb2222|cccc3333|Periodic key rotation|1700000000"
	if payload != expected {
		t.Fatalf("expected payload %q, got %q", expected, payload)
	}
}

