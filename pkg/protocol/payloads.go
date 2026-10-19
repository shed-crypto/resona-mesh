package protocol

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
