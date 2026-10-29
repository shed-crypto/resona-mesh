package p2p

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"resonamesh/internal/updater"
	"resonamesh/pkg/protocol"
)

const (
	ReleaseStreamTimeout = 15 * time.Second
	MaxReleaseMsgSize    = 1024 * 1024 // 1 MiB max
)

const (
	ActionAnnounceRelease = "ANNOUNCE"
	ActionQueryRelease    = "QUERY"
)

// ReleaseWireRequest is the message exchanged over /resonamesh/release/1.0.0.
type ReleaseWireRequest struct {
	Action   string                    `json:"action"` // ANNOUNCE or QUERY
	Manifest *protocol.ReleaseManifest `json:"manifest,omitempty"`
}

// ReleaseWireResponse is returned over /resonamesh/release/1.0.0.
type ReleaseWireResponse struct {
	Success        bool                      `json:"success"`
	Message        string                    `json:"message,omitempty"`
	LatestManifest *protocol.ReleaseManifest `json:"latest_manifest,omitempty"`
}

// ReleaseHandler coordinates P2P release propagation over libp2p.
type ReleaseHandler struct {
	host    *Host
	updater *updater.Engine
	mu      sync.RWMutex
}

// NewReleaseHandler registers the release protocol on the libp2p host.
func NewReleaseHandler(h *Host, u *updater.Engine) *ReleaseHandler {
	rh := &ReleaseHandler{
		host:    h,
		updater: u,
	}

	h.Raw().SetStreamHandler(protocol.ProtocolRelease, rh.handleStream)
	return rh
}

func (rh *ReleaseHandler) handleStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ReleaseStreamTimeout))

	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		slog.Debug("Release stream failed to read request length", "err", err)
		return
	}

	if reqLen > MaxReleaseMsgSize {
		_ = rh.writeError(s, "request exceeds maximum size")
		return
	}

	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		slog.Debug("Release stream failed to read request body", "err", err)
		return
	}

	var req ReleaseWireRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		slog.Debug("Release stream failed to parse json payload", "err", err)
		_ = rh.writeError(s, "invalid json payload")
		return
	}

	var resp ReleaseWireResponse

	switch req.Action {
	case ActionAnnounceRelease:
		if req.Manifest == nil {
			_ = rh.writeError(s, "missing manifest payload")
			return
		}

		accepted, err := rh.updater.ProcessAnnouncedRelease(req.Manifest)
		if err != nil {
			resp = ReleaseWireResponse{
				Success: false,
				Message: err.Error(),
			}
		} else {
			resp = ReleaseWireResponse{
				Success: true,
				Message: fmt.Sprintf("release announcement processed (accepted=%v)", accepted),
			}
		}

	case ActionQueryRelease:
		state := rh.updater.State()
		resp = ReleaseWireResponse{
			Success:        true,
			LatestManifest: state.AvailableRelease,
		}

	default:
		_ = rh.writeError(s, fmt.Sprintf("unsupported action %q", req.Action))
		return
	}

	_ = rh.writeResponse(s, &resp)
}

// AnnounceRelease sends a verified release manifest to a single peer.
func (rh *ReleaseHandler) AnnounceRelease(ctx context.Context, peerID peer.ID, rel *protocol.ReleaseManifest) error {
	s, err := rh.host.NewControlStream(ctx, peerID, protocol.ProtocolRelease)
	if err != nil {
		return fmt.Errorf("failed to open release stream to peer %s: %w", peerID, err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ReleaseStreamTimeout))

	req := ReleaseWireRequest{
		Action:   ActionAnnounceRelease,
		Manifest: rel,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal announce request: %w", err)
	}

	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBytes))); err != nil {
		return err
	}
	if _, err := s.Write(reqBytes); err != nil {
		return err
	}

	var respLen uint32
	if err := binary.Read(s, binary.BigEndian, &respLen); err != nil {
		return err
	}
	if respLen > MaxReleaseMsgSize {
		return errors.New("response exceeds maximum size")
	}

	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		return err
	}

	var resp ReleaseWireResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return fmt.Errorf("invalid response json: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("peer rejected announcement: %s", resp.Message)
	}

	return nil
}

// BroadcastRelease announces a new release manifest to all currently connected peers.
func (rh *ReleaseHandler) BroadcastRelease(ctx context.Context, rel *protocol.ReleaseManifest) (int, error) {
	peers := rh.host.Raw().Network().Peers()
	if len(peers) == 0 {
		return 0, nil
	}

	count := 0
	var lastErr error

	for _, p := range peers {
		if err := rh.AnnounceRelease(ctx, p, rel); err != nil {
			slog.Debug("Failed to announce release to peer", "peer", p, "err", err)
			lastErr = err
		} else {
			count++
		}
	}

	return count, lastErr
}

// QueryPeer queries a peer for its latest available release manifest.
func (rh *ReleaseHandler) QueryPeer(ctx context.Context, peerID peer.ID) (*protocol.ReleaseManifest, error) {
	s, err := rh.host.NewControlStream(ctx, peerID, protocol.ProtocolRelease)
	if err != nil {
		return nil, fmt.Errorf("failed to open release query stream: %w", err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ReleaseStreamTimeout))

	req := ReleaseWireRequest{
		Action: ActionQueryRelease,
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	if err := binary.Write(s, binary.BigEndian, uint32(len(reqBytes))); err != nil {
		return nil, err
	}
	if _, err := s.Write(reqBytes); err != nil {
		return nil, err
	}

	var respLen uint32
	if err := binary.Read(s, binary.BigEndian, &respLen); err != nil {
		return nil, err
	}
	if respLen > MaxReleaseMsgSize {
		return nil, errors.New("response exceeds maximum size")
	}

	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		return nil, err
	}

	var resp ReleaseWireResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, fmt.Errorf("invalid query response json: %w", err)
	}

	if resp.LatestManifest != nil {
		// Process received release manifest
		_, _ = rh.updater.ProcessAnnouncedRelease(resp.LatestManifest)
	}

	return resp.LatestManifest, nil
}

// QueryAllPeers queries all connected swarm peers for releases.
func (rh *ReleaseHandler) QueryAllPeers(ctx context.Context) (int, error) {
	peers := rh.host.Raw().Network().Peers()
	if len(peers) == 0 {
		return 0, nil
	}

	discovered := 0
	for _, p := range peers {
		manifest, err := rh.QueryPeer(ctx, p)
		if err == nil && manifest != nil {
			discovered++
		}
	}

	return discovered, nil
}

func (rh *ReleaseHandler) writeError(s network.Stream, msg string) error {
	resp := ReleaseWireResponse{
		Success: false,
		Message: msg,
	}
	return rh.writeResponse(s, &resp)
}

func (rh *ReleaseHandler) writeResponse(s network.Stream, resp *ReleaseWireResponse) error {
	respBytes, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	if err := binary.Write(s, binary.BigEndian, uint32(len(respBytes))); err != nil {
		return err
	}
	_, err = s.Write(respBytes)
	return err
}
