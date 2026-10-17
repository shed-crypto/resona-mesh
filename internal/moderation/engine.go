package moderation

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"resonamesh/internal/storage"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

var (
	ErrReportNotFound     = errors.New("moderation: report not found")
	ErrUntrustedModerator = errors.New("moderation: untrusted moderator public key")
	ErrInvalidSignature   = errors.New("moderation: invalid cryptographic signature")
	ErrInvalidReasonCode  = errors.New("moderation: invalid reason code")
	ErrMissingContentID   = errors.New("moderation: content ID is required")
	ErrForkDetected       = errors.New("moderation: sequence fork or conflicting history detected")
	ErrSequenceGap        = errors.New("moderation: sequence gap detected in decision chain")
)

type Config struct {
	NetworkID              string
	PolicyID               string
	PolicyEpoch            uint64
	TrustedModeratorPubKey string
}

type Engine struct {
	db             *storage.DB
	cfg            Config
	trustedKey     ed25519.PublicKey
	nodePrivateKey ed25519.PrivateKey
	mu             sync.RWMutex
}

type CreateReportParams struct {
	ContentID         string
	PayloadHash       string
	ReasonCode        string
	Description       string
	EvidenceReference string
}

type CreateDecisionParams struct {
	ContentID   string
	PayloadHash string
	Action      string
	ReasonCode  string
	ReasonText  string
}

type ReportRecord struct {
	EventID           string `json:"event_id"`
	ContentID         string `json:"content_id"`
	PayloadHash       string `json:"payload_hash"`
	ReporterKey       string `json:"reporter_key"`
	ReasonCode        string `json:"reason_code"`
	Description       string `json:"description"`
	EvidenceRef       string `json:"evidence_ref"`
	CreatedAt         int64  `json:"created_at"`
	Nonce             string `json:"nonce"`
	Signature         string `json:"signature"`
	Status            string `json:"status"`
	ReceiptSignature  string `json:"receipt_signature"`
	ReceiptReceivedAt int64  `json:"receipt_received_at"`
}

type DecisionRecord struct {
	EventID         string `json:"event_id"`
	PolicyID        string `json:"policy_id"`
	PolicyEpoch     uint64 `json:"policy_epoch"`
	ContentID       string `json:"content_id"`
	PayloadHash     string `json:"payload_hash"`
	Sequence        uint64 `json:"sequence"`
	PreviousEventID string `json:"previous_event_id"`
	Action          string `json:"action"`
	ReasonCode      string `json:"reason_code"`
	ReasonText      string `json:"reason_text"`
	ModeratorKey    string `json:"moderator_key"`
	IssuedAt        int64  `json:"issued_at"`
	Signature       string `json:"signature"`
}

type BlockedRecord struct {
	ContentID       string `json:"content_id"`
	PayloadHash     string `json:"payload_hash"`
	Action          string `json:"action"`
	DecisionEventID string `json:"decision_event_id"`
	ReasonCode      string `json:"reason_code"`
	ReasonText      string `json:"reason_text"`
	BlockedAt       int64  `json:"blocked_at"`
}

func NewEngine(db *storage.DB, cfg Config, nodeKey ed25519.PrivateKey) (*Engine, error) {
	if cfg.NetworkID == "" {
		cfg.NetworkID = "resonamesh-main"
	}
	if cfg.PolicyID == "" {
		cfg.PolicyID = "policy-default"
	}
	if cfg.PolicyEpoch == 0 {
		cfg.PolicyEpoch = 1
	}

	var trustedPub ed25519.PublicKey
	if cfg.TrustedModeratorPubKey != "" {
		pub, err := crypto.HexToPublicKey(cfg.TrustedModeratorPubKey)
		if err != nil {
			return nil, fmt.Errorf("moderation: invalid trusted moderator key: %w", err)
		}
		trustedPub = pub
	}

	return &Engine{
		db:             db,
		cfg:            cfg,
		trustedKey:     trustedPub,
		nodePrivateKey: nodeKey,
	}, nil
}

// SetTrustedModeratorKey updates the trusted moderator public key in memory.
func (e *Engine) SetTrustedModeratorKey(pub ed25519.PublicKey) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.trustedKey = pub
	e.cfg.TrustedModeratorPubKey = crypto.PublicKeyToHex(pub)
}

// SubmitReport creates, signs, and persists an abuse report into the local outbox.
func (e *Engine) SubmitReport(ctx context.Context, params CreateReportParams) (*protocol.ReportEvent, error) {
	if strings.TrimSpace(params.ContentID) == "" {
		return nil, ErrMissingContentID
	}

	switch params.ReasonCode {
	case protocol.ReasonCopyrightClaim, protocol.ReasonLicenseMismatch,
		protocol.ReasonMisleadingMetadata, protocol.ReasonOther:
	default:
		return nil, ErrInvalidReasonCode
	}

	if e.nodePrivateKey == nil {
		return nil, errors.New("moderation: node private key is required to sign report")
	}

	pubKey := e.nodePrivateKey.Public().(ed25519.PublicKey)
	reporterKeyHex := crypto.PublicKeyToHex(pubKey)

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("moderation: failed to generate nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	now := time.Now().Unix()

	report := &protocol.ReportEvent{
		SchemaVersion:     "1.0.0",
		NetworkID:         e.cfg.NetworkID,
		PolicyID:          e.cfg.PolicyID,
		ContentID:         params.ContentID,
		PayloadHash:       params.PayloadHash,
		ReporterKeyHex:    reporterKeyHex,
		ReasonCode:        params.ReasonCode,
		Description:       params.Description,
		EvidenceReference: params.EvidenceReference,
		CreatedAt:         now,
		Nonce:             nonce,
	}

	canonicalPayload := protocol.CanonicalReportPayload(report)
	sigBytes := crypto.SignWithDomain(e.nodePrivateKey, crypto.DomainReport, []byte(canonicalPayload))
	report.Signature = hex.EncodeToString(sigBytes)

	eventID := protocol.ReportEventID(report)

	query := `
	INSERT INTO reports (
		event_id, content_id, payload_hash, reporter_key, reason_code,
		description, evidence_ref, created_at, nonce, signature, status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'PENDING')`

	_, err := e.db.Raw().ExecContext(ctx, query,
		eventID,
		report.ContentID,
		report.PayloadHash,
		report.ReporterKeyHex,
		report.ReasonCode,
		report.Description,
		report.EvidenceReference,
		report.CreatedAt,
		report.Nonce,
		report.Signature,
	)
	if err != nil {
		return nil, fmt.Errorf("moderation: failed to persist report in outbox: %w", err)
	}

	return report, nil
}

// RecordReceipt records a signed moderator receipt acknowledging receipt of a report.
func (e *Engine) RecordReceipt(ctx context.Context, rc *protocol.ReceiptEvent) error {
	e.mu.RLock()
	trustedKey := e.trustedKey
	e.mu.RUnlock()

	if trustedKey != nil {
		if !strings.EqualFold(crypto.PublicKeyToHex(trustedKey), rc.ModeratorKeyHex) {
			return ErrUntrustedModerator
		}
	}

	modPubKey, err := crypto.HexToPublicKey(rc.ModeratorKeyHex)
	if err != nil {
		return fmt.Errorf("moderation: invalid moderator public key in receipt: %w", err)
	}

	sigBytes, err := hex.DecodeString(rc.Signature)
	if err != nil {
		return ErrInvalidSignature
	}

	canonicalPayload := protocol.CanonicalReceiptPayload(rc)
	if !crypto.VerifyWithDomain(modPubKey, crypto.DomainReceipt, []byte(canonicalPayload), sigBytes) {
		return ErrInvalidSignature
	}

	query := `
	UPDATE reports 
	SET status = 'ACCEPTED', receipt_signature = ?, receipt_received_at = ?
	WHERE event_id = ?`

	res, err := e.db.Raw().ExecContext(ctx, query, rc.Signature, rc.ReceivedAt, rc.ReportID)
	if err != nil {
		return fmt.Errorf("moderation: failed to record receipt: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrReportNotFound
	}

	return nil
}

// ApplyDecision verifies, orders, and applies an incoming moderation decision.
func (e *Engine) ApplyDecision(ctx context.Context, d *protocol.DecisionEvent) error {
	e.mu.RLock()
	trustedKey := e.trustedKey
	e.mu.RUnlock()

	// 1. Verify moderator key trust
	if trustedKey != nil {
		if !strings.EqualFold(crypto.PublicKeyToHex(trustedKey), d.ModeratorKeyHex) {
			return ErrUntrustedModerator
		}
	}

	modPubKey, err := crypto.HexToPublicKey(d.ModeratorKeyHex)
	if err != nil {
		return fmt.Errorf("moderation: invalid moderator key: %w", err)
	}

	// 2. Verify signature over canonical payload
	sigBytes, err := hex.DecodeString(d.Signature)
	if err != nil {
		return ErrInvalidSignature
	}

	canonicalPayload := protocol.CanonicalDecisionPayload(d)
	if !crypto.VerifyWithDomain(modPubKey, crypto.DomainDecision, []byte(canonicalPayload), sigBytes) {
		return ErrInvalidSignature
	}

	eventID := protocol.DecisionEventID(d)

	// 3. Monotonic sequencing and fork detection
	tx, err := e.db.Raw().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("moderation: failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if already applied
	var existingCount int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM decisions WHERE event_id = ?", eventID).Scan(&existingCount)
	if err != nil {
		return err
	}
	if existingCount > 0 {
		return nil // Idempotent skip
	}

	// Find the latest decision for this target to check sequence continuity
	var lastSeq uint64
	var lastEventID string
	queryLatest := `
	SELECT sequence, event_id 
	FROM decisions 
	WHERE policy_id = ? AND policy_epoch = ? AND content_id = ?
	ORDER BY sequence DESC LIMIT 1`

	row := tx.QueryRowContext(ctx, queryLatest, d.PolicyID, d.PolicyEpoch, d.ContentID)
	err = row.Scan(&lastSeq, &lastEventID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("moderation: failed to query sequence state: %w", err)
	}

	var hasGap bool
	if errors.Is(err, sql.ErrNoRows) {
		if d.Sequence != 1 {
			hasGap = true
		}
		if d.Sequence == 1 && d.PreviousEventID != "" {
			return ErrForkDetected
		}
	} else {
		// We have previous decisions
		if d.Sequence <= lastSeq {
			return ErrForkDetected
		}
		if d.Sequence > lastSeq+1 {
			hasGap = true
		} else if d.PreviousEventID != lastEventID {
			return ErrForkDetected
		}
	}

	// 4. Record decision
	insertDecisionSQL := `
	INSERT INTO decisions (
		event_id, policy_id, policy_epoch, content_id, payload_hash,
		sequence, previous_event_id, action, reason_code, reason_text,
		moderator_key, issued_at, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, insertDecisionSQL,
		eventID,
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
		d.Signature,
	)
	if err != nil {
		return fmt.Errorf("moderation: failed to store decision: %w", err)
	}

	// 5. Update blocked_content state
	switch d.Action {
	case protocol.ActionBlock:
		upsertBlockedSQL := `
		INSERT INTO blocked_content (
			content_id, payload_hash, action, decision_event_id,
			reason_code, reason_text, blocked_at
		) VALUES (?, ?, 'BLOCK', ?, ?, ?, ?)
		ON CONFLICT(content_id) DO UPDATE SET
			payload_hash = excluded.payload_hash,
			action = 'BLOCK',
			decision_event_id = excluded.decision_event_id,
			reason_code = excluded.reason_code,
			reason_text = excluded.reason_text,
			blocked_at = excluded.blocked_at`

		_, err = tx.ExecContext(ctx, upsertBlockedSQL,
			d.ContentID,
			d.PayloadHash,
			eventID,
			d.ReasonCode,
			d.ReasonText,
			d.IssuedAt,
		)
		if err != nil {
			return fmt.Errorf("moderation: failed to update blocked content: %w", err)
		}

	case protocol.ActionRestore, protocol.ActionClear:
		deleteBlockedSQL := `DELETE FROM blocked_content WHERE content_id = ?`
		if d.PayloadHash != "" {
			deleteBlockedSQL = `DELETE FROM blocked_content WHERE content_id = ? OR payload_hash = ?`
			_, err = tx.ExecContext(ctx, deleteBlockedSQL, d.ContentID, d.PayloadHash)
		} else {
			_, err = tx.ExecContext(ctx, deleteBlockedSQL, d.ContentID)
		}
		if err != nil {
			return fmt.Errorf("moderation: failed to remove blocked content: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("moderation: failed to commit decision transaction: %w", err)
	}

	if hasGap {
		return ErrSequenceGap
	}

	return nil
}

// IsBlocked performs dual matching against content_id and payload_hash.
func (e *Engine) IsBlocked(ctx context.Context, contentID string, payloadHash string) (bool, string, error) {
	if contentID == "" && payloadHash == "" {
		return false, "", nil
	}

	query := `
	SELECT reason_text 
	FROM blocked_content 
	WHERE (content_id != '' AND content_id = ?) 
	   OR (? != '' AND payload_hash = ?)
	LIMIT 1`

	var reason string
	err := e.db.Raw().QueryRowContext(ctx, query, contentID, payloadHash, payloadHash).Scan(&reason)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, "", nil
		}
		return false, "", fmt.Errorf("moderation: failed to check block status: %w", err)
	}

	return true, reason, nil
}

// CreateDecision creates, signs, and applies a decision if node has the moderator private key.
func (e *Engine) CreateDecision(ctx context.Context, modPrivKey ed25519.PrivateKey, params CreateDecisionParams) (*protocol.DecisionEvent, error) {
	if modPrivKey == nil {
		return nil, errors.New("moderation: moderator private key is required")
	}

	modPubKey := modPrivKey.Public().(ed25519.PublicKey)
	modKeyHex := crypto.PublicKeyToHex(modPubKey)

	// Determine next sequence and previous event ID
	var lastSeq uint64
	var lastEventID string
	queryLatest := `
	SELECT sequence, event_id 
	FROM decisions 
	WHERE policy_id = ? AND policy_epoch = ? AND content_id = ?
	ORDER BY sequence DESC LIMIT 1`

	row := e.db.Raw().QueryRowContext(ctx, queryLatest, e.cfg.PolicyID, e.cfg.PolicyEpoch, params.ContentID)
	err := row.Scan(&lastSeq, &lastEventID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("moderation: failed to query sequence state: %w", err)
	}

	nextSeq := lastSeq + 1
	prevID := lastEventID
	now := time.Now().Unix()

	decision := &protocol.DecisionEvent{
		SchemaVersion:   "1.0.0",
		NetworkID:       e.cfg.NetworkID,
		PolicyID:        e.cfg.PolicyID,
		PolicyEpoch:     e.cfg.PolicyEpoch,
		ContentID:       params.ContentID,
		PayloadHash:     params.PayloadHash,
		Sequence:        nextSeq,
		PreviousEventID: prevID,
		Action:          params.Action,
		ReasonCode:      params.ReasonCode,
		ReasonText:      params.ReasonText,
		ModeratorKeyHex: modKeyHex,
		IssuedAt:        now,
	}

	canonicalPayload := protocol.CanonicalDecisionPayload(decision)
	sigBytes := crypto.SignWithDomain(modPrivKey, crypto.DomainDecision, []byte(canonicalPayload))
	decision.Signature = hex.EncodeToString(sigBytes)

	if err := e.ApplyDecision(ctx, decision); err != nil && !errors.Is(err, ErrSequenceGap) {
		return nil, fmt.Errorf("moderation: failed to apply generated decision: %w", err)
	}

	return decision, nil
}

// CreateReceipt creates and signs a receipt acknowledging a report.
func (e *Engine) CreateReceipt(ctx context.Context, modPrivKey ed25519.PrivateKey, reportID string) (*protocol.ReceiptEvent, error) {
	if modPrivKey == nil {
		return nil, errors.New("moderation: moderator private key is required")
	}

	modPubKey := modPrivKey.Public().(ed25519.PublicKey)
	modKeyHex := crypto.PublicKeyToHex(modPubKey)
	now := time.Now().Unix()

	receipt := &protocol.ReceiptEvent{
		ReportID:        reportID,
		ModeratorKeyHex: modKeyHex,
		ReceivedAt:      now,
	}

	canonical := protocol.CanonicalReceiptPayload(receipt)
	sigBytes := crypto.SignWithDomain(modPrivKey, crypto.DomainReceipt, []byte(canonical))
	receipt.Signature = hex.EncodeToString(sigBytes)

	return receipt, nil
}

// ListReports returns reports stored in the local outbox.
func (e *Engine) ListReports(ctx context.Context) ([]ReportRecord, error) {
	query := `
	SELECT 
		event_id, content_id, payload_hash, reporter_key, reason_code,
		description, evidence_ref, created_at, nonce, signature, status,
		receipt_signature, receipt_received_at
	FROM reports
	ORDER BY created_at DESC`

	rows, err := e.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("moderation: failed to list reports: %w", err)
	}
	defer rows.Close()

	var records []ReportRecord
	for rows.Next() {
		var r ReportRecord
		if err := rows.Scan(
			&r.EventID, &r.ContentID, &r.PayloadHash, &r.ReporterKey, &r.ReasonCode,
			&r.Description, &r.EvidenceRef, &r.CreatedAt, &r.Nonce, &r.Signature,
			&r.Status, &r.ReceiptSignature, &r.ReceiptReceivedAt,
		); err != nil {
			return nil, fmt.Errorf("moderation: failed to scan report row: %w", err)
		}
		records = append(records, r)
	}

	return records, nil
}

// ListDecisions returns decisions stored locally.
func (e *Engine) ListDecisions(ctx context.Context) ([]DecisionRecord, error) {
	query := `
	SELECT 
		event_id, policy_id, policy_epoch, content_id, payload_hash,
		sequence, previous_event_id, action, reason_code, reason_text,
		moderator_key, issued_at, signature
	FROM decisions
	ORDER BY issued_at DESC, sequence DESC`

	rows, err := e.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("moderation: failed to list decisions: %w", err)
	}
	defer rows.Close()

	var records []DecisionRecord
	for rows.Next() {
		var d DecisionRecord
		if err := rows.Scan(
			&d.EventID, &d.PolicyID, &d.PolicyEpoch, &d.ContentID, &d.PayloadHash,
			&d.Sequence, &d.PreviousEventID, &d.Action, &d.ReasonCode, &d.ReasonText,
			&d.ModeratorKey, &d.IssuedAt, &d.Signature,
		); err != nil {
			return nil, fmt.Errorf("moderation: failed to scan decision row: %w", err)
		}
		records = append(records, d)
	}

	return records, nil
}

// ListBlocked returns all currently blocked content records.
func (e *Engine) ListBlocked(ctx context.Context) ([]BlockedRecord, error) {
	query := `
	SELECT 
		content_id, payload_hash, action, decision_event_id,
		reason_code, reason_text, blocked_at
	FROM blocked_content
	ORDER BY blocked_at DESC`

	rows, err := e.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("moderation: failed to list blocked content: %w", err)
	}
	defer rows.Close()

	var records []BlockedRecord
	for rows.Next() {
		var b BlockedRecord
		if err := rows.Scan(
			&b.ContentID, &b.PayloadHash, &b.Action, &b.DecisionEventID,
			&b.ReasonCode, &b.ReasonText, &b.BlockedAt,
		); err != nil {
			return nil, fmt.Errorf("moderation: failed to scan blocked row: %w", err)
		}
		records = append(records, b)
	}

	return records, nil
}
