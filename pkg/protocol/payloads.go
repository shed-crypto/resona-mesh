package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// CanonicalReportPayload produces a deterministic string representation of a report for signing.
func CanonicalReportPayload(r *ReportEvent) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%s",
		r.SchemaVersion,
		r.NetworkID,
		r.PolicyID,
		r.ContentID,
		r.PayloadHash,
		r.ReporterKeyHex,
		r.ReasonCode,
		r.Description,
		r.EvidenceReference,
		r.CreatedAt,
		r.Nonce,
	)
}

// ReportEventID computes the SHA-256 identifier of a canonical report payload.
func ReportEventID(r *ReportEvent) string {
	sum := sha256.Sum256([]byte(CanonicalReportPayload(r)))
	return hex.EncodeToString(sum[:])
}

// CanonicalDecisionPayload produces a deterministic string representation of a decision for signing.
func CanonicalDecisionPayload(d *DecisionEvent) string {
	return fmt.Sprintf("%s|%s|%s|%d|%s|%s|%d|%s|%s|%s|%s|%s|%d",
		d.SchemaVersion,
		d.NetworkID,
		d.PolicyID,
		d.PolicyEpoch,
		d.ContentID,
		d.PayloadHash,
		d.Sequence,
		d.PreviousEventID,
		d.Action,
		d.ReasonCode,
		d.ReasonText,
		d.ModeratorKeyHex,
		d.IssuedAt,
	)
}

// DecisionEventID computes the SHA-256 identifier of a canonical decision payload.
func DecisionEventID(d *DecisionEvent) string {
	sum := sha256.Sum256([]byte(CanonicalDecisionPayload(d)))
	return hex.EncodeToString(sum[:])
}

// CanonicalReceiptPayload produces a deterministic string representation of a receipt for signing.
func CanonicalReceiptPayload(rc *ReceiptEvent) string {
	return fmt.Sprintf("%s|%s|%d",
		rc.ReportID,
		rc.ModeratorKeyHex,
		rc.ReceivedAt,
	)
}

// CanonicalModeratorCertPayload produces a deterministic string representation of a moderator certificate.
func CanonicalModeratorCertPayload(c *ModeratorCertificate) string {
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s",
		c.NetworkID,
		c.Action,
		strings.ToLower(c.ModeratorPubKey),
		c.Alias,
		c.IssuedAt,
		c.ExpiresAt,
		strings.ToLower(c.RootAdminPubKey),
	)
}

// CanonicalKeyRotationPayload produces a deterministic string representation of a key rotation event.
func CanonicalKeyRotationPayload(r *KeyRotationEvent) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%s|%s|%d",
		r.NetworkID,
		r.KeyRole,
		r.NewPolicyEpoch,
		strings.ToLower(r.OldPubKey),
		strings.ToLower(r.NewPubKey),
		strings.ToLower(r.RecoveryPubKey),
		r.Reason,
		r.IssuedAt,
	)
}


// CanonicalReleasePayload produces a deterministic string representation of a release manifest for signing.
func CanonicalReleasePayload(r *ReleaseManifest) string {
	hashes := strings.Join(r.BlockHashes, ",")
	return fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%s|%s|%d|%s",
		r.SchemaVersion,
		r.Version,
		r.TargetOS,
		r.TargetArch,
		strings.ToLower(r.BinaryHash),
		r.TotalSizeBytes,
		r.BlockSize,
		hashes,
		strings.ToLower(r.ReleasePubKey),
		r.PublishedAt,
		r.ReleaseNotes,
	)
}

// ReleaseManifestID computes the SHA-256 identifier of a canonical release payload.
func ReleaseManifestID(r *ReleaseManifest) string {
	sum := sha256.Sum256([]byte(CanonicalReleasePayload(r)))
	return hex.EncodeToString(sum[:])
}
