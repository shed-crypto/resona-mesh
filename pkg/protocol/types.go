package protocol

// Manifest describes a verified, signed audio track and its constituent chunks.
type Manifest struct {
	SchemaVersion   string   `json:"schema_version"`
	ContentID       string   `json:"content_id"`
	Title           string   `json:"title"`
	Artist          string   `json:"artist"`
	DurationMs      int64    `json:"duration_ms"`
	BitrateKbps     int      `json:"bitrate_kbps"`
	TotalSizeBytes  int64    `json:"total_size_bytes"`
	BlockSize       int      `json:"block_size"`
	BlockHashes     []string `json:"block_hashes"`
	PublisherKeyHex string   `json:"publisher_key_hex"`
	PublishedAt     int64    `json:"published_at"`
	LicenseCode     string   `json:"license_code"`
	Signature       string   `json:"signature"`
	Lyrics          string   `json:"lyrics,omitempty"`
}

// BlockRequest requests an individual 256 KiB audio chunk by index.
type BlockRequest struct {
	ContentID  string `json:"content_id"`
	BlockIndex int    `json:"block_index"`
}

// BlockResponse contains the payload of the requested block with its verification hash.
type BlockResponse struct {
	ContentID  string `json:"content_id"`
	BlockIndex int    `json:"block_index"`
	BlockHash  string `json:"block_hash"`
	Data       []byte `json:"data"`
}

// DecisionEvent represents a cryptographically signed community moderation decision.
type DecisionEvent struct {
	SchemaVersion   string `json:"schema_version"`
	NetworkID       string `json:"network_id"`
	PolicyID        string `json:"policy_id"`
	PolicyEpoch     uint64 `json:"policy_epoch"`
	ContentID       string `json:"content_id"`
	PayloadHash     string `json:"payload_hash,omitempty"`
	Sequence        uint64 `json:"sequence"`
	PreviousEventID string `json:"previous_event_id"`
	Action          string `json:"action"`
	ReasonCode      string `json:"reason_code"`
	ReasonText      string `json:"reason_text"`
	ModeratorKeyHex string `json:"moderator_key_hex"`
	IssuedAt        int64  `json:"issued_at"`
	Signature       string `json:"signature"`
}

// ReportEvent represents an authenticated abuse or copyright report submitted to moderation.
type ReportEvent struct {
	SchemaVersion     string `json:"schema_version"`
	NetworkID         string `json:"network_id"`
	PolicyID          string `json:"policy_id"`
	ContentID         string `json:"content_id"`
	PayloadHash       string `json:"payload_hash,omitempty"`
	ReporterKeyHex    string `json:"reporter_key_hex"`
	ReasonCode        string `json:"reason_code"`
	Description       string `json:"description"`
	EvidenceReference string `json:"evidence_reference"`
	CreatedAt         int64  `json:"created_at"`
	Nonce             string `json:"nonce"`
	Signature         string `json:"signature"`
}

// ReceiptEvent represents a cryptographically signed confirmation by a moderator that a report was received.
type ReceiptEvent struct {
	ReportID        string `json:"report_id"`
	ModeratorKeyHex string `json:"moderator_key_hex"`
	ReceivedAt      int64  `json:"received_at"`
	Signature       string `json:"signature"`
}

// ModeratorCertificate delegates or revokes moderation authority from the Root Admin to a moderator key.
type ModeratorCertificate struct {
	CertID          string `json:"cert_id"`
	NetworkID       string `json:"network_id"`
	Action          string `json:"action"` // "APPROVE_MODERATOR" or "REVOKE_MODERATOR"
	ModeratorPubKey string `json:"moderator_pubkey"`
	Alias           string `json:"alias,omitempty"`
	IssuedAt        int64  `json:"issued_at"`
	ExpiresAt       int64  `json:"expires_at,omitempty"` // 0 = no expiry
	RootAdminPubKey string `json:"root_admin_pubkey"`
	Signature       string `json:"signature"`
}

// KeyRotationEvent replaces a lost or compromised Root Admin or Release key, signed by the offline Recovery Key.
type KeyRotationEvent struct {
	EventID        string `json:"event_id"`
	NetworkID      string `json:"network_id"`
	KeyRole        string `json:"key_role"` // "ROOT_ADMIN" or "RELEASE"
	NewPolicyEpoch uint64 `json:"new_policy_epoch"`
	OldPubKey      string `json:"old_pubkey"`
	NewPubKey      string `json:"new_pubkey"`
	RecoveryPubKey string `json:"recovery_pubkey"`
	Reason         string `json:"reason"`
	IssuedAt       int64  `json:"issued_at"`
	Signature      string `json:"signature"`
}

// ReleaseManifest describes an authentic, signed application update binary chunked across the swarm.
type ReleaseManifest struct {
	SchemaVersion  string   `json:"schema_version"` // "resonamesh:release:v1"
	Version        string   `json:"version"`        // e.g. "0.2.0"
	TargetOS       string   `json:"target_os"`     // "windows", "linux", "darwin"
	TargetArch     string   `json:"target_arch"`   // "amd64", "arm64"
	BinaryHash     string   `json:"binary_hash"`   // SHA-256 of monolithic executable
	TotalSizeBytes int64    `json:"total_size_bytes"`
	BlockSize      int      `json:"block_size"`    // 262144 (256 KiB)
	BlockHashes    []string `json:"block_hashes"`  // Ordered block hashes
	ReleasePubKey  string   `json:"release_pubkey"`
	PublishedAt    int64    `json:"published_at"`
	ReleaseNotes   string   `json:"release_notes,omitempty"`
	Signature      string   `json:"signature"`
}
