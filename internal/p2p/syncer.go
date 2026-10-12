package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"resonamesh/internal/importer"
	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

const (
	// MaxManifestPayloadSize limits incoming manifest array payload (4 MiB).
	MaxManifestPayloadSize = 4 * 1024 * 1024

	// ManifestStreamTimeout limits individual manifest sync operations.
	ManifestStreamTimeout = 15 * time.Second
)

var (
	ErrInvalidManifestSignature = errors.New("p2p: manifest ed25519 signature is invalid")
	ErrManifestIntegrityFailed  = errors.New("p2p: manifest content id does not match composite chunk hashes")
	ErrNilCatalog               = errors.New("p2p: catalog syncer requires non-nil catalog")
)

// ManifestCatalog defines the catalog interface required for manifest synchronization.
type ManifestCatalog interface {
	ListManifests(ctx context.Context) ([]*protocol.Manifest, error)
	GetManifest(ctx context.Context, contentID string) (*protocol.Manifest, error)
	RegisterRemoteManifest(ctx context.Context, m *protocol.Manifest) error
}

// ManifestSyncRequest represents the wire request for manifest synchronization.
type ManifestSyncRequest struct {
	Action    string `json:"action"` // "LIST" or "GET"
	ContentID string `json:"content_id,omitempty"`
}

// CatalogSyncer manages distributed catalog metadata exchange and cryptographic validation.
type CatalogSyncer struct {
	host    *Host
	catalog ManifestCatalog
}

// NewCatalogSyncer binds the manifest synchronization protocol to the libp2p host.
func NewCatalogSyncer(h *Host, cat ManifestCatalog) (*CatalogSyncer, error) {
	if cat == nil {
		return nil, ErrNilCatalog
	}

	cs := &CatalogSyncer{
		host:    h,
		catalog: cat,
	}

	h.Raw().SetStreamHandler(protocol.ProtocolManifest, cs.handleStream)
	return cs, nil
}

// handleStream serves incoming manifest requests from swarm peers.
func (cs *CatalogSyncer) handleStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ManifestStreamTimeout))

	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		slog.Debug("Failed to read manifest request length", "err", err)
		return
	}

	if reqLen > MaxBlockRequestSize {
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		slog.Debug("Failed to read manifest request body", "err", err)
		return
	}

	var req ManifestSyncRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		slog.Debug("Failed to unmarshal manifest request json", "err", err)
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ManifestStreamTimeout)
	defer cancel()

	var payload []byte
	var err error

	switch req.Action {
	case "LIST":
		manifests, listErr := cs.catalog.ListManifests(ctx)
		if listErr != nil {
			_ = binary.Write(s, binary.BigEndian, StatusError)
			return
		}
		if manifests == nil {
			manifests = []*protocol.Manifest{}
		}
		payload, err = json.Marshal(manifests)
	case "GET":
		manifest, getErr := cs.catalog.GetManifest(ctx, req.ContentID)
		if getErr != nil {
			_ = binary.Write(s, binary.BigEndian, StatusNotFound)
			return
		}
		payload, err = json.Marshal(manifest)
	default:
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	if err != nil {
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	if err := binary.Write(s, binary.BigEndian, StatusSuccess); err != nil {
		return
	}
	if err := binary.Write(s, binary.BigEndian, uint32(len(payload))); err != nil {
		return
	}
	_, _ = s.Write(payload)
}

// SyncFromPeer requests all known manifests from a remote peer, verifies their Ed25519 signatures,
// and registers unseeded metadata into the local catalog.
func (cs *CatalogSyncer) SyncFromPeer(ctx context.Context, peerID peer.ID) (int, error) {
	s, err := cs.host.Raw().NewStream(ctx, peerID, protocol.ProtocolManifest)
	if err != nil {
		return 0, fmt.Errorf("p2p: failed to open manifest stream to %s: %w", peerID, err)
	}
	defer s.Close()

	_ = s.SetDeadline(time.Now().Add(ManifestStreamTimeout))

	reqBytes, err := json.Marshal(ManifestSyncRequest{Action: "LIST"})
	if err != nil {
		return 0, err
	}

	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBytes))); err != nil {
		return 0, err
	}
	if _, err := s.Write(reqBytes); err != nil {
		return 0, err
	}

	var status byte
	if err := binary.Read(s, binary.BigEndian, &status); err != nil {
		return 0, err
	}
	if status != StatusSuccess {
		return 0, fmt.Errorf("p2p: peer returned status %x for manifest list", status)
	}

	var payloadLen uint32
	if err := binary.Read(s, binary.BigEndian, &payloadLen); err != nil {
		return 0, err
	}
	if payloadLen > MaxManifestPayloadSize {
		return 0, errors.New("p2p: manifest payload exceeds max allowed size")
	}

	payloadBuf := make([]byte, payloadLen)
	if _, err := io.ReadFull(s, payloadBuf); err != nil {
		return 0, err
	}

	var manifests []*protocol.Manifest
	if err := json.Unmarshal(payloadBuf, &manifests); err != nil {
		return 0, fmt.Errorf("p2p: failed to decode manifest json: %w", err)
	}

	importedCount := 0
	for _, m := range manifests {
		if err := VerifyManifest(m); err != nil {
			slog.Warn("Skipping corrupt or unverified remote manifest",
				"peer_id", peerID.String(),
				"content_id", m.ContentID,
				"error", err,
			)
			continue
		}

		if err := cs.catalog.RegisterRemoteManifest(ctx, m); err != nil {
			slog.Warn("Failed to persist verified remote manifest to catalog",
				"content_id", m.ContentID,
				"error", err,
			)
			continue
		}
		importedCount++
	}

	slog.Info("Completed catalog sync with swarm peer",
		"peer_id", peerID.String(),
		"total_received", len(manifests),
		"imported", importedCount,
	)

	return importedCount, nil
}

// VerifyManifest checks both composite SHA-256 content ID integrity and Ed25519 signature validity.
func VerifyManifest(m *protocol.Manifest) error {
	if m == nil {
		return errors.New("p2p: manifest is nil")
	}
	if len(m.BlockHashes) == 0 {
		return errors.New("p2p: manifest contains no block hashes")
	}

	// 1. Verify content ID is composite SHA-256 of block hashes
	expectedID := crypto.HashContent(m.BlockHashes)
	if m.ContentID != expectedID {
		return fmt.Errorf("%w: expected %s, got %s", ErrManifestIntegrityFailed, expectedID, m.ContentID)
	}

	// 2. Decode publisher public key
	pubKeyBytes, err := hex.DecodeString(m.PublisherKeyHex)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("p2p: invalid publisher public key: %w", err)
	}

	// 3. Decode signature
	sigBytes, err := hex.DecodeString(m.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("p2p: invalid signature format: %w", err)
	}

	// 4. Verify cryptographic signature with domain separation
	canonicalPayload := importer.CanonicalManifestPayload(m)
	if !crypto.VerifyWithDomain(ed25519.PublicKey(pubKeyBytes), crypto.DomainManifest, []byte(canonicalPayload), sigBytes) {
		return ErrInvalidManifestSignature
	}

	return nil
}
