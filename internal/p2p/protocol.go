package p2p

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"resonamesh/pkg/crypto"
	"resonamesh/pkg/protocol"
)

const (
	// Wire status codes
	StatusSuccess  byte = 0x00
	StatusNotFound byte = 0x01
	StatusError    byte = 0x02
	StatusBlocked  byte = 0x03

	// MaxBlockRequestSize prevents memory exhaustion on malformed request headers.
	MaxBlockRequestSize = 64 * 1024

	// StreamTimeout limits the duration of an individual block wire exchange.
	StreamTimeout = 10 * time.Second
)

var (
	ErrBlockNotFound      = errors.New("p2p: requested block not found on remote peer")
	ErrRemotePeerError    = errors.New("p2p: remote peer encountered error serving block")
	ErrContentBlocked     = errors.New("p2p: content is blocked by moderation policy")
	ErrInvalidStatus      = errors.New("p2p: invalid wire status code from peer")
	ErrBlockHashMismatch  = errors.New("p2p: received block failed SHA-256 integrity verification")
	ErrBlockSizeExceeded  = errors.New("p2p: received block exceeds maximum allowed block size")
	ErrEmptyBlockExpected = errors.New("p2p: expected block hash cannot be empty")
)

// WireBlockRequest defines the wire schema requested over a libp2p block stream.
type WireBlockRequest struct {
	ContentID  string `json:"content_id"`
	BlockIndex int    `json:"block_index"`
	BlockHash  string `json:"block_hash"`
}

// BlockProvider provides access to local blocks to serve to swarm peers.
type BlockProvider interface {
	GetBlock(ctx context.Context, contentID string, blockIndex int, blockHash string) ([]byte, error)
}

// ProtocolHandler registers and serves the ResonaMesh block exchange protocol.
type ProtocolHandler struct {
	host         *Host
	provider     BlockProvider
	blockChecker BlockChecker
}

// NewProtocolHandler binds the block exchange wire protocol to the libp2p host.
func NewProtocolHandler(h *Host, provider BlockProvider) *ProtocolHandler {
	ph := &ProtocolHandler{
		host:     h,
		provider: provider,
	}

	h.Raw().SetStreamHandler(protocol.ProtocolBlock, ph.handleStream)
	return ph
}

// SetBlockChecker binds a moderation policy checker to guard against serving blocked content over wire.
func (ph *ProtocolHandler) SetBlockChecker(bc BlockChecker) {
	ph.blockChecker = bc
}

// handleStream handles incoming stream requests from remote peers.
func (ph *ProtocolHandler) handleStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(StreamTimeout))

	// 1. Read request length prefix
	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		slog.Debug("Failed to read block request length", "err", err)
		return
	}

	if reqLen > MaxBlockRequestSize {
		slog.Warn("Block request exceeds max allowed size", "size", reqLen)
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	// 2. Read JSON request body
	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		slog.Debug("Failed to read block request body", "err", err)
		return
	}

	var req WireBlockRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		slog.Debug("Failed to parse block request json", "err", err)
		_ = binary.Write(s, binary.BigEndian, StatusError)
		return
	}

	// 3. Query provider for block data
	ctx, cancel := context.WithTimeout(context.Background(), StreamTimeout)
	defer cancel()

	if ph.blockChecker != nil {
		blocked, _, err := ph.blockChecker.IsBlocked(ctx, req.ContentID, "")
		if err == nil && blocked {
			_ = binary.Write(s, binary.BigEndian, StatusBlocked)
			return
		}
	}

	data, err := ph.provider.GetBlock(ctx, req.ContentID, req.BlockIndex, req.BlockHash)
	if err != nil {
		slog.Warn("Block requested by swarm peer not found locally", "peer_id", s.Conn().RemotePeer().String(), "hash", req.BlockHash)
		_ = binary.Write(s, binary.BigEndian, StatusNotFound)
		return
	}

	// 4. Send StatusSuccess + length + payload
	if err := binary.Write(s, binary.BigEndian, StatusSuccess); err != nil {
		return
	}

	if err := binary.Write(s, binary.BigEndian, uint32(len(data))); err != nil {
		return
	}

	if _, err := s.Write(data); err != nil {
		slog.Warn("Failed to write block payload to stream", "peer_id", s.Conn().RemotePeer().String(), "err", err)
		return
	}
	slog.Info("Served audio block to swarm peer", "peer_id", s.Conn().RemotePeer().String(), "hash", req.BlockHash, "bytes", len(data))
}

// RequestBlock fetches an individual block from a remote peer over libp2p.
func RequestBlock(
	ctx context.Context,
	h *Host,
	peerID peer.ID,
	req WireBlockRequest,
) ([]byte, error) {
	if req.BlockHash == "" {
		return nil, ErrEmptyBlockExpected
	}

	s, err := h.Raw().NewStream(ctx, peerID, protocol.ProtocolBlock)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to open block stream to %s: %w", peerID, err)
	}
	defer s.Close()

	_ = s.SetDeadline(time.Now().Add(StreamTimeout))

	// 1. Serialize request
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to marshal block request: %w", err)
	}

	// 2. Write length prefix + JSON request
	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBytes))); err != nil {
		return nil, fmt.Errorf("p2p: failed to write request length: %w", err)
	}

	if _, err := s.Write(reqBytes); err != nil {
		return nil, fmt.Errorf("p2p: failed to write request body: %w", err)
	}

	// 3. Read status byte
	var status byte
	if err := binary.Read(s, binary.BigEndian, &status); err != nil {
		return nil, fmt.Errorf("p2p: failed to read response status: %w", err)
	}

	switch status {
	case StatusNotFound:
		return nil, ErrBlockNotFound
	case StatusBlocked:
		return nil, ErrContentBlocked
	case StatusError:
		return nil, ErrRemotePeerError
	case StatusSuccess:
		// continue to payload reading
	default:
		return nil, ErrInvalidStatus
	}

	// 4. Read payload length
	var payloadLen uint32
	if err := binary.Read(s, binary.BigEndian, &payloadLen); err != nil {
		return nil, fmt.Errorf("p2p: failed to read payload length: %w", err)
	}

	if payloadLen > protocol.BlockSize {
		return nil, ErrBlockSizeExceeded
	}

	// 5. Read block data
	buf := make([]byte, payloadLen)
	if _, err := io.ReadFull(s, buf); err != nil {
		return nil, fmt.Errorf("p2p: failed to read full block data: %w", err)
	}

	// 6. Verify SHA-256 integrity against expected hash
	if !crypto.VerifyChunk(buf, req.BlockHash) {
		return nil, ErrBlockHashMismatch
	}

	return buf, nil
}
