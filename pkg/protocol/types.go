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
	ContentID       string `json:"content_id"`
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
	ReporterKeyHex    string `json:"reporter_key_hex"`
	ReasonCode        string `json:"reason_code"`
	Description       string `json:"description"`
	EvidenceReference string `json:"evidence_reference"`
	CreatedAt         int64  `json:"created_at"`
	Nonce             string `json:"nonce"`
	Signature         string `json:"signature"`
}
