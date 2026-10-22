package protocol

const (
	// BlockSize is the canonical chunk size for ResonaMesh audio distribution: 256 KiB.
	BlockSize = 256 * 1024

	// MaxManifestSizeBytes is the maximum allowed size for a track manifest (1 MiB).
	MaxManifestSizeBytes = 1024 * 1024

	// Default networking ports
	DefaultHTTPPort = 8080
	DefaultP2PPort  = 4001

	// libp2p wire protocol identifiers
	ProtocolBlock      = "/resonamesh/block/1.0.0"
	ProtocolManifest   = "/resonamesh/manifest/1.0.0"
	ProtocolDiscovery  = "/resonamesh/discovery/1.0.0"
	ProtocolModeration = "/resonamesh/moderation/1.0.0"

	// Moderation action types
	ActionBlock   = "BLOCK"
	ActionClear   = "CLEAR"
	ActionRestore = "RESTORE"

	// Moderation report reason codes
	ReasonCopyrightClaim     = "COPYRIGHT_CLAIM"
	ReasonLicenseMismatch    = "LICENSE_MISMATCH"
	ReasonMisleadingMetadata = "MISLEADING_METADATA"
	ReasonOther              = "OTHER"

	// Supported open licenses
	LicenseCC0    = "CC0"
	LicenseCCBY40 = "CC-BY-4.0"
)
