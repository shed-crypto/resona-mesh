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
	RecoveryPubKey         string
}

type Engine struct {
	db             *storage.DB
	cfg            Config
	trustedKey        ed25519.PublicKey
	currentReleaseKey string
	nodePrivateKey ed25519.PrivateKey
	mu             sync.RWMutex
}

type CreateReportParams struct {
	ContentID         string `json:"content_id"`
	PayloadHash       string `json:"payload_hash"`
	ReasonCode        string `json:"reason_code"`
	Description       string `json:"description"`
	EvidenceReference string `json:"evidence_reference"`
}

type CreateDecisionParams struct {
	ContentID   string `json:"content_id"`
	PayloadHash string `json:"payload_hash"`
	Action      string `json:"action"`
	ReasonCode  string `json:"reason_code"`
	ReasonText  string `json:"reason_text"`
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
	SchemaVersion   string `json:"schema_version"`
	NetworkID       string `json:"network_id"`
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

// Config returns a copy of the engine configuration.
func (e *Engine) Config() Config {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.cfg
}

// StoreReceivedReport persists an incoming authenticated report from another peer.
func (e *Engine) StoreReceivedReport(ctx context.Context, r *protocol.ReportEvent) error {
	eventID := protocol.ReportEventID(r)
	query := `
	INSERT OR IGNORE INTO reports (
		event_id, content_id, payload_hash, reporter_key, reason_code,
		description, evidence_ref, created_at, nonce, signature, status
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'RECEIVED')`

	_, err := e.db.Raw().ExecContext(ctx, query,
		eventID,
		r.ContentID,
		r.PayloadHash,
		r.ReporterKeyHex,
		r.ReasonCode,
		r.Description,
		r.EvidenceReference,
		r.CreatedAt,
		r.Nonce,
		r.Signature,
	)
	return err
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




	// 1. Verify moderator key trust
	if !e.isTrustedModerator(ctx, d.ModeratorKeyHex) {
		return ErrUntrustedModerator
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
	schemaVer := d.SchemaVersion
	if schemaVer == "" {
		schemaVer = "1.0.0"
	}
	netID := d.NetworkID
	if netID == "" {
		netID = e.cfg.NetworkID
	}

	insertDecisionSQL := `
	INSERT INTO decisions (
		event_id, schema_version, network_id, policy_id, policy_epoch, content_id, payload_hash,
		sequence, previous_event_id, action, reason_code, reason_text,
		moderator_key, issued_at, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = tx.ExecContext(ctx, insertDecisionSQL,
		eventID,
		schemaVer,
		netID,
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
		event_id, schema_version, network_id, policy_id, policy_epoch, content_id, payload_hash,
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
			&d.EventID, &d.SchemaVersion, &d.NetworkID, &d.PolicyID, &d.PolicyEpoch, &d.ContentID, &d.PayloadHash,
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

// isTrustedModerator evaluates whether a given hex public key is authorized to sign
// decisions and receipts under the current Root Admin and active delegation certificates.
func (e *Engine) isTrustedModerator(ctx context.Context, keyHex string) bool {
	e.mu.RLock()
	trustedRoot := e.trustedKey
	e.mu.RUnlock()

	// Open mode: if no root key is configured, all keys are accepted
	if trustedRoot == nil {
		return true
	}

	rootHex := crypto.PublicKeyToHex(trustedRoot)
	if strings.EqualFold(rootHex, keyHex) {
		return true
	}

	// Query latest certificate for keyHex
	query := `
	SELECT action, expires_at, root_admin_pubkey, signature,
	       network_id, moderator_pubkey, alias, issued_at
	FROM moderator_certificates
	WHERE LOWER(moderator_pubkey) = LOWER(?)
	ORDER BY issued_at DESC
	LIMIT 1`

	var action, rootAdmin, sig, netID, modPub, alias string
	var expiresAt, issuedAt int64
	err := e.db.Raw().QueryRowContext(ctx, query, keyHex).Scan(
		&action, &expiresAt, &rootAdmin, &sig,
		&netID, &modPub, &alias, &issuedAt,
	)
	if err != nil {
		return false
	}

	if !strings.EqualFold(rootAdmin, rootHex) {
		return false
	}
	if action != "APPROVE_MODERATOR" {
		return false
	}
	if expiresAt > 0 && expiresAt <= time.Now().Unix() {
		return false
	}

	rootPubBytes, err := crypto.HexToPublicKey(rootAdmin)
	if err != nil {
		return false
	}
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	cert := &protocol.ModeratorCertificate{
		NetworkID:       netID,
		Action:          action,
		ModeratorPubKey: modPub,
		Alias:           alias,
		IssuedAt:        issuedAt,
		ExpiresAt:       expiresAt,
		RootAdminPubKey: rootAdmin,
	}
	payload := protocol.CanonicalModeratorCertPayload(cert)
	return crypto.VerifyWithDomain(rootPubBytes, crypto.DomainModeratorCert, []byte(payload), sigBytes)
}

// StoreModeratorCertificate verifies and persists an incoming delegation certificate.
func (e *Engine) StoreModeratorCertificate(ctx context.Context, cert *protocol.ModeratorCertificate) error {
	e.mu.RLock()
	trustedRoot := e.trustedKey
	e.mu.RUnlock()

	if trustedRoot != nil {
		rootHex := crypto.PublicKeyToHex(trustedRoot)
		if !strings.EqualFold(rootHex, cert.RootAdminPubKey) {
			return ErrUntrustedModerator
		}
	}

	rootPubBytes, err := crypto.HexToPublicKey(cert.RootAdminPubKey)
	if err != nil {
		return fmt.Errorf("moderation: invalid root admin key: %w", err)
	}

	sigBytes, err := hex.DecodeString(cert.Signature)
	if err != nil {
		return ErrInvalidSignature
	}

	payload := protocol.CanonicalModeratorCertPayload(cert)
	if !crypto.VerifyWithDomain(rootPubBytes, crypto.DomainModeratorCert, []byte(payload), sigBytes) {
		return ErrInvalidSignature
	}

	certID := cert.CertID
	if certID == "" {
		certID = crypto.HashChunkHex([]byte(payload))
		cert.CertID = certID
	}

	query := `
	INSERT OR IGNORE INTO moderator_certificates (
		cert_id, network_id, action, moderator_pubkey, alias,
		issued_at, expires_at, root_admin_pubkey, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err = e.db.Raw().ExecContext(ctx, query,
		cert.CertID, cert.NetworkID, cert.Action, cert.ModeratorPubKey, cert.Alias,
		cert.IssuedAt, cert.ExpiresAt, cert.RootAdminPubKey, cert.Signature,
	)
	return err
}

// DelegateModerator creates, signs, and stores a delegation certificate for a moderator.
func (e *Engine) DelegateModerator(ctx context.Context, action, modPubKeyHex, alias string, durationSec int64) (*protocol.ModeratorCertificate, error) {
	e.mu.RLock()
	trustedRoot := e.trustedKey
	e.mu.RUnlock()

	if trustedRoot == nil {
		return nil, errors.New("moderation: no trusted root admin configured on node")
	}
	if e.nodePrivateKey == nil {
		return nil, errors.New("moderation: node private key is required to sign delegation")
	}

	localPub := e.nodePrivateKey.Public().(ed25519.PublicKey)
	if !strings.EqualFold(crypto.PublicKeyToHex(localPub), crypto.PublicKeyToHex(trustedRoot)) {
		return nil, errors.New("moderation: only active root admin can delegate moderators")
	}

	now := time.Now().Unix()
	var expiresAt int64
	if durationSec > 0 {
		expiresAt = now + durationSec
	}

	cert := &protocol.ModeratorCertificate{
		NetworkID:       e.cfg.NetworkID,
		Action:          action,
		ModeratorPubKey: modPubKeyHex,
		Alias:           alias,
		IssuedAt:        now,
		ExpiresAt:       expiresAt,
		RootAdminPubKey: crypto.PublicKeyToHex(trustedRoot),
	}

	payload := protocol.CanonicalModeratorCertPayload(cert)
	sig := crypto.SignWithDomain(e.nodePrivateKey, crypto.DomainModeratorCert, []byte(payload))
	cert.Signature = hex.EncodeToString(sig)
	cert.CertID = crypto.HashChunkHex([]byte(payload))

	if err := e.StoreModeratorCertificate(ctx, cert); err != nil {
		return nil, err
	}
	return cert, nil
}

// ApplyKeyRotation validates and persists an emergency key rotation signed by the Recovery Key.
func (e *Engine) ApplyKeyRotation(ctx context.Context, rot *protocol.KeyRotationEvent) error {
	e.mu.RLock()
	expectedRecovery := e.cfg.RecoveryPubKey
	e.mu.RUnlock()

	if expectedRecovery == "" {
		return errors.New("moderation: no recovery public key configured to authorize rotation")
	}
	if !strings.EqualFold(expectedRecovery, rot.RecoveryPubKey) {
		return errors.New("moderation: recovery key mismatch in rotation event")
	}

	recPubBytes, err := crypto.HexToPublicKey(rot.RecoveryPubKey)
	if err != nil {
		return fmt.Errorf("moderation: invalid recovery public key: %w", err)
	}

	sigBytes, err := hex.DecodeString(rot.Signature)
	if err != nil {
		return ErrInvalidSignature
	}

	payload := protocol.CanonicalKeyRotationPayload(rot)
	if !crypto.VerifyWithDomain(recPubBytes, crypto.DomainKeyRotation, []byte(payload), sigBytes) {
		return ErrInvalidSignature
	}

	// Verify epoch is strictly greater than highest stored epoch for this key role
	var maxEpoch sql.NullInt64
	err = e.db.Raw().QueryRowContext(ctx, "SELECT MAX(new_policy_epoch) FROM key_rotations WHERE key_role = ?", rot.KeyRole).Scan(&maxEpoch)
	if err == nil && maxEpoch.Valid && uint64(maxEpoch.Int64) >= rot.NewPolicyEpoch {
		return fmt.Errorf("moderation: new policy epoch %d must be greater than current %d", rot.NewPolicyEpoch, maxEpoch.Int64)
	}

	eventID := rot.EventID
	if eventID == "" {
		eventID = crypto.HashChunkHex([]byte(payload))
		rot.EventID = eventID
	}

	query := `
	INSERT INTO key_rotations (
		event_id, network_id, key_role, new_policy_epoch,
		old_pubkey, new_pubkey, recovery_pubkey, reason, issued_at, signature
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	if _, err := e.db.Raw().ExecContext(ctx, query,
		rot.EventID, rot.NetworkID, rot.KeyRole, rot.NewPolicyEpoch,
		rot.OldPubKey, rot.NewPubKey, rot.RecoveryPubKey, rot.Reason, rot.IssuedAt, rot.Signature,
	); err != nil {
		return fmt.Errorf("moderation: failed to persist key rotation: %w", err)
	}

	// Apply rotation to active runtime state
	e.mu.Lock()
	defer e.mu.Unlock()
	switch rot.KeyRole {
	case "ROOT_ADMIN":
		newPub, err := crypto.HexToPublicKey(rot.NewPubKey)
		if err == nil {
			e.trustedKey = newPub
			e.cfg.TrustedModeratorPubKey = rot.NewPubKey
			e.cfg.PolicyEpoch = rot.NewPolicyEpoch
		}
	case "RELEASE":
		e.currentReleaseKey = rot.NewPubKey
	}

	return nil
}

// CurrentReleaseKey returns the active release signing public key.
func (e *Engine) CurrentReleaseKey() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.currentReleaseKey
}

// SetCurrentReleaseKey sets the active release public key.
func (e *Engine) SetCurrentReleaseKey(pubHex string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.currentReleaseKey = pubHex
}

// ListModeratorCertificates returns stored moderator certificates.
func (e *Engine) ListModeratorCertificates(ctx context.Context) ([]protocol.ModeratorCertificate, error) {
	query := `
	SELECT cert_id, network_id, action, moderator_pubkey, alias,
	       issued_at, expires_at, root_admin_pubkey, signature
	FROM moderator_certificates
	ORDER BY issued_at DESC`

	rows, err := e.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certs []protocol.ModeratorCertificate
	for rows.Next() {
		var c protocol.ModeratorCertificate
		if err := rows.Scan(
			&c.CertID, &c.NetworkID, &c.Action, &c.ModeratorPubKey, &c.Alias,
			&c.IssuedAt, &c.ExpiresAt, &c.RootAdminPubKey, &c.Signature,
		); err != nil {
			return nil, err
		}
		certs = append(certs, c)
	}
	return certs, nil
}

// ListKeyRotations returns all stored key rotation events.
func (e *Engine) ListKeyRotations(ctx context.Context) ([]protocol.KeyRotationEvent, error) {
	query := `
	SELECT event_id, network_id, key_role, new_policy_epoch,
	       old_pubkey, new_pubkey, recovery_pubkey, reason, issued_at, signature
	FROM key_rotations
	ORDER BY new_policy_epoch DESC`

	rows, err := e.db.Raw().QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rots []protocol.KeyRotationEvent
	for rows.Next() {
		var r protocol.KeyRotationEvent
		if err := rows.Scan(
			&r.EventID, &r.NetworkID, &r.KeyRole, &r.NewPolicyEpoch,
			&r.OldPubKey, &r.NewPubKey, &r.RecoveryPubKey, &r.Reason, &r.IssuedAt, &r.Signature,
		); err != nil {
			return nil, err
		}
		rots = append(rots, r)
	}
	return rots, nil
}

func (e *Engine) loadPersistedRotations(ctx context.Context) error {
	// Replay latest ROOT_ADMIN rotation
	var newRootPub string
	var rootEpoch uint64
	err := e.db.Raw().QueryRowContext(ctx, `
		SELECT new_pubkey, new_policy_epoch FROM key_rotations
		WHERE key_role = 'ROOT_ADMIN'
		ORDER BY new_policy_epoch DESC LIMIT 1`).Scan(&newRootPub, &rootEpoch)
	if err == nil && newRootPub != "" {
		if pubBytes, err := crypto.HexToPublicKey(newRootPub); err == nil {
			e.trustedKey = pubBytes
			e.cfg.TrustedModeratorPubKey = newRootPub
			e.cfg.PolicyEpoch = rootEpoch
		}
	}

	// Replay latest RELEASE rotation
	var newReleasePub string
	err = e.db.Raw().QueryRowContext(ctx, `
		SELECT new_pubkey FROM key_rotations
		WHERE key_role = 'RELEASE'
		ORDER BY new_policy_epoch DESC LIMIT 1`).Scan(&newReleasePub)
	if err == nil && newReleasePub != "" {
		e.currentReleaseKey = newReleasePub
	}

	return nil
}
