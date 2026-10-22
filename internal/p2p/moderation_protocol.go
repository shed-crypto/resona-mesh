package p2p

import (
	"context"
	"crypto/ed25519"
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

	"resonamesh/internal/moderation"
	"resonamesh/pkg/protocol"
)

const (
	ActionSubmitReport  = "SUBMIT_REPORT"
	ActionSyncDecisions = "SYNC_DECISIONS"

	ModerationStreamTimeout = 15 * time.Second
	MaxModerationMsgSize    = 256 * 1024 // 256 KiB
)

var (
	ErrModerationStreamFailed = errors.New("p2p: moderation stream operation failed")
)

type ModerationWireRequest struct {
	Action        string                `json:"action"`
	Report        *protocol.ReportEvent `json:"report,omitempty"`
	SinceSequence uint64                `json:"since_sequence,omitempty"`
}

type ModerationWireResponse struct {
	Status    byte                      `json:"status"` // 0x00 OK, 0x01 Error
	Receipt   *protocol.ReceiptEvent    `json:"receipt,omitempty"`
	Decisions []*protocol.DecisionEvent `json:"decisions,omitempty"`
	Error     string                    `json:"error,omitempty"`
}

// ModerationHandler handles incoming streams for /resonamesh/moderation/1.0.0
type ModerationHandler struct {
	host           *Host
	engine         *moderation.Engine
	modPrivKey     ed25519.PrivateKey
	mu             sync.RWMutex
	running        bool
	stopCh         chan struct{}
	doneCh         chan struct{}
	syncInterval   time.Duration
}

// NewModerationHandler creates and registers the moderation wire protocol handler.
func NewModerationHandler(h *Host, eng *moderation.Engine, modKey ed25519.PrivateKey) *ModerationHandler {
	mh := &ModerationHandler{
		host:         h,
		engine:       eng,
		modPrivKey:   modKey,
		syncInterval: 30 * time.Second,
	}

	h.Raw().SetStreamHandler(protocol.ProtocolModeration, mh.handleStream)
	return mh
}

// SetModeratorPrivateKey sets or updates the local moderator signing key.
func (mh *ModerationHandler) SetModeratorPrivateKey(priv ed25519.PrivateKey) {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	mh.modPrivKey = priv
}

func (mh *ModerationHandler) handleStream(s network.Stream) {
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ModerationStreamTimeout))

	var reqLen uint32
	if err := binary.Read(s, binary.BigEndian, &reqLen); err != nil {
		slog.Debug("Moderation stream failed to read request length", "err", err)
		return
	}

	if reqLen > MaxModerationMsgSize {
		_ = mh.writeError(s, "request exceeds maximum size")
		return
	}

	reqBuf := make([]byte, reqLen)
	if _, err := io.ReadFull(s, reqBuf); err != nil {
		slog.Debug("Moderation stream failed to read request body", "err", err)
		return
	}

	var req ModerationWireRequest
	if err := json.Unmarshal(reqBuf, &req); err != nil {
		_ = mh.writeError(s, "invalid json payload")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), ModerationStreamTimeout)
	defer cancel()

	var resp ModerationWireResponse
	switch req.Action {
	case ActionSubmitReport:
		if req.Report == nil {
			_ = mh.writeError(s, "missing report payload")
			return
		}

		// Persist report into local outbox / store
		_ = mh.engine.StoreReceivedReport(ctx, req.Report)

		mh.mu.RLock()
		modKey := mh.modPrivKey
		mh.mu.RUnlock()

		// If this node is a moderator, sign and return a receipt
		if modKey != nil {
			repID := protocol.ReportEventID(req.Report)
			receipt, err := mh.engine.CreateReceipt(ctx, modKey, repID)
			if err == nil {
				resp.Status = 0x00
				resp.Receipt = receipt
			} else {
				resp.Status = 0x00
			}
		} else {
			resp.Status = 0x00
		}

	case ActionSyncDecisions:
		decisions, err := mh.engine.ListDecisions(ctx)
		if err != nil {
			_ = mh.writeError(s, err.Error())
			return
		}

		var filtered []*protocol.DecisionEvent
		for _, d := range decisions {
			if d.Sequence > req.SinceSequence {
				decEvent := &protocol.DecisionEvent{
					SchemaVersion:   d.SchemaVersion,
					NetworkID:       d.NetworkID,
					PolicyID:        d.PolicyID,
					PolicyEpoch:     d.PolicyEpoch,
					ContentID:       d.ContentID,
					PayloadHash:     d.PayloadHash,
					Sequence:        d.Sequence,
					PreviousEventID: d.PreviousEventID,
					Action:          d.Action,
					ReasonCode:      d.ReasonCode,
					ReasonText:      d.ReasonText,
					ModeratorKeyHex: d.ModeratorKey,
					IssuedAt:        d.IssuedAt,
					Signature:       d.Signature,
				}
				filtered = append(filtered, decEvent)
			}
		}
		resp.Status = 0x00
		resp.Decisions = filtered

	default:
		_ = mh.writeError(s, "unknown action")
		return
	}

	_ = mh.writeResponse(s, resp)
}

func (mh *ModerationHandler) writeError(s network.Stream, msg string) error {
	return mh.writeResponse(s, ModerationWireResponse{
		Status: 0x01,
		Error:  msg,
	})
}

func (mh *ModerationHandler) writeResponse(s network.Stream, resp ModerationWireResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if err := binary.Write(s, binary.BigEndian, uint32(len(data))); err != nil {
		return err
	}
	_, err = s.Write(data)
	return err
}

// SendReport sends a report to a remote peer and returns any receipt.
func (mh *ModerationHandler) SendReport(ctx context.Context, peerID peer.ID, report *protocol.ReportEvent) (*protocol.ReceiptEvent, error) {
	s, err := mh.host.Raw().NewStream(ctx, peerID, protocol.ProtocolModeration)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to open moderation stream to %s: %w", peerID, err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ModerationStreamTimeout))

	req := ModerationWireRequest{
		Action: ActionSubmitReport,
		Report: report,
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

	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		return nil, err
	}

	var resp ModerationWireResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, err
	}

	if resp.Status != 0x00 {
		return nil, fmt.Errorf("p2p: peer reported moderation error: %s", resp.Error)
	}

	return resp.Receipt, nil
}

// SyncDecisions pulls newer decisions from a remote peer starting from sinceSeq.
func (mh *ModerationHandler) SyncDecisions(ctx context.Context, peerID peer.ID, sinceSeq uint64) ([]*protocol.DecisionEvent, error) {
	s, err := mh.host.Raw().NewStream(ctx, peerID, protocol.ProtocolModeration)
	if err != nil {
		return nil, fmt.Errorf("p2p: failed to open moderation stream to %s: %w", peerID, err)
	}
	defer s.Close()
	_ = s.SetDeadline(time.Now().Add(ModerationStreamTimeout))

	req := ModerationWireRequest{
		Action:        ActionSyncDecisions,
		SinceSequence: sinceSeq,
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

	respBuf := make([]byte, respLen)
	if _, err := io.ReadFull(s, respBuf); err != nil {
		return nil, err
	}

	var resp ModerationWireResponse
	if err := json.Unmarshal(respBuf, &resp); err != nil {
		return nil, err
	}

	if resp.Status != 0x00 {
		return nil, fmt.Errorf("p2p: peer reported decision sync error: %s", resp.Error)
	}

	return resp.Decisions, nil
}

// SyncCycle runs an iteration of pushing pending reports and fetching decisions across connected peers.
func (mh *ModerationHandler) SyncCycle(ctx context.Context) error {
	peers := mh.host.ConnectedPeers()
	if len(peers) == 0 {
		return nil
	}

	// 1. Push pending reports
	reports, err := mh.engine.ListReports(ctx)
	if err == nil {
		for _, r := range reports {
			if r.Status == "PENDING" {
				cfg := mh.engine.Config()
				repEvent := &protocol.ReportEvent{
					SchemaVersion:     "1.0.0",
					NetworkID:         cfg.NetworkID,
					PolicyID:          cfg.PolicyID,
					ContentID:         r.ContentID,
					PayloadHash:       r.PayloadHash,
					ReporterKeyHex:    r.ReporterKey,
					ReasonCode:        r.ReasonCode,
					Description:       r.Description,
					EvidenceReference: r.EvidenceRef,
					CreatedAt:         r.CreatedAt,
					Nonce:             r.Nonce,
					Signature:         r.Signature,
				}

				for _, p := range peers {
					receipt, sendErr := mh.SendReport(ctx, p, repEvent)
					if sendErr == nil && receipt != nil {
						_ = mh.engine.RecordReceipt(ctx, receipt)
						break
					}
				}
			}
		}
	}

	// 2. Pull decisions
	decisions, err := mh.engine.ListDecisions(ctx)
	var maxSeq uint64
	if err == nil && len(decisions) > 0 {
		for _, d := range decisions {
			if d.Sequence > maxSeq {
				maxSeq = d.Sequence
			}
		}
	}

	for _, p := range peers {
		remoteDecisions, syncErr := mh.SyncDecisions(ctx, p, maxSeq)
		if syncErr == nil && len(remoteDecisions) > 0 {
			for _, rd := range remoteDecisions {
				_ = mh.engine.ApplyDecision(ctx, rd)
			}
		}
	}

	return nil
}

// Start begins periodic background store-and-forward moderation sync.
func (mh *ModerationHandler) Start(ctx context.Context) {
	mh.mu.Lock()
	if mh.running {
		mh.mu.Unlock()
		return
	}
	mh.running = true
	mh.stopCh = make(chan struct{})
	mh.doneCh = make(chan struct{})
	mh.mu.Unlock()

	go func() {
		defer close(mh.doneCh)
		ticker := time.NewTicker(mh.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-mh.stopCh:
				return
			case <-ticker.C:
				_ = mh.SyncCycle(ctx)
			}
		}
	}()
}

// Stop halts background synchronization cleanly.
func (mh *ModerationHandler) Stop() {
	mh.mu.Lock()
	if !mh.running {
		mh.mu.Unlock()
		return
	}
	mh.running = false
	close(mh.stopCh)
	mh.mu.Unlock()
	<-mh.doneCh
}
