package p2p

import (
	"math"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// FailureType classifies the category of an observed peer transfer failure.
type FailureType int

const (
	// FailureStreamError represents general wire protocol or socket disconnection errors.
	FailureStreamError FailureType = iota

	// FailureTimeout represents an expired read/write deadline during block transfer.
	FailureTimeout

	// FailureHashMismatch represents a critical SHA-256 integrity checksum violation.
	FailureHashMismatch
)

const (
	// DefaultQuarantineBaseDuration is the initial quarantine period for SHA-256 integrity failures.
	DefaultQuarantineBaseDuration = 10 * time.Minute

	// MaxQuarantineDuration caps the exponential doubling of quarantine penalties.
	MaxQuarantineDuration = 24 * time.Hour

	// TimeoutQuarantineDuration is the quarantine duration applied after consecutive timeouts.
	TimeoutQuarantineDuration = 1 * time.Minute

	// ConsecutiveTimeoutThreshold is the number of sequential timeouts that trigger temporary quarantine.
	ConsecutiveTimeoutThreshold = 3

	// LatencyAlpha is the smoothing factor for the exponential moving average of round-trip latency.
	LatencyAlpha = 0.2

	// BaseReputationScore is the initial score assigned to peers with no transfer history.
	BaseReputationScore = 100.0

	// MaxBlockVerificationBonus caps the maximum score increase attainable from verified blocks.
	MaxBlockVerificationBonus = 50.0
)

// PeerTransferStats holds runtime reliability, throughput, and quarantine state for a swarm peer.
type PeerTransferStats struct {
	PeerID              peer.ID   `json:"peer_id"`
	BlocksVerified      int64     `json:"blocks_verified"`
	HashMismatchCount   int64     `json:"hash_mismatch_count"`
	StreamErrors        int64     `json:"stream_errors"`
	ConsecutiveTimeouts int       `json:"consecutive_timeouts"`
	AvgLatencyMs        float64   `json:"avg_latency_ms"`
	QuarantinedUntil    time.Time `json:"quarantined_until,omitempty"`
	QuarantineCount     int       `json:"quarantine_count"`
	LastSeen            time.Time `json:"last_seen"`
}

// PeerReputationTracker provides thread-safe accounting of peer transfer metrics, latency EMA, and quarantine state.
type PeerReputationTracker struct {
	mu    sync.RWMutex
	stats map[peer.ID]*PeerTransferStats
}

// NewPeerReputationTracker instantiates an empty reputation registry.
func NewPeerReputationTracker() *PeerReputationTracker {
	return &PeerReputationTracker{
		stats: make(map[peer.ID]*PeerTransferStats),
	}
}

// RecordSuccess records a successful block verification and updates the exponential moving average latency.
func (rt *PeerReputationTracker) RecordSuccess(peerID peer.ID, elapsed time.Duration, bytesReceived int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	s := rt.getOrCreateLocked(peerID)
	s.BlocksVerified++
	s.ConsecutiveTimeouts = 0

	latencyMs := float64(elapsed.Microseconds()) / 1000.0
	if latencyMs <= 0 {
		latencyMs = 0.001
	}

	if s.AvgLatencyMs <= 0 {
		s.AvgLatencyMs = latencyMs
	} else {
		s.AvgLatencyMs = (1.0-LatencyAlpha)*s.AvgLatencyMs + LatencyAlpha*latencyMs
	}
	s.LastSeen = time.Now()
}

// RecordFailure records a transfer anomaly and applies quarantine rules according to the failure classification.
func (rt *PeerReputationTracker) RecordFailure(peerID peer.ID, failureType FailureType) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	s := rt.getOrCreateLocked(peerID)
	now := time.Now()
	s.LastSeen = now

	switch failureType {
	case FailureHashMismatch:
		s.HashMismatchCount++
		s.ConsecutiveTimeouts = 0

		// Exponential doubling: 10m, 20m, 40m... capped at 24h
		mult := 1
		if s.QuarantineCount > 0 && s.QuarantineCount < 30 {
			mult = 1 << s.QuarantineCount
		}
		duration := DefaultQuarantineBaseDuration * time.Duration(mult)
		if duration > MaxQuarantineDuration {
			duration = MaxQuarantineDuration
		}

		s.QuarantinedUntil = now.Add(duration)
		s.QuarantineCount++

	case FailureTimeout:
		s.StreamErrors++
		s.ConsecutiveTimeouts++
		if s.ConsecutiveTimeouts >= ConsecutiveTimeoutThreshold {
			s.QuarantinedUntil = now.Add(TimeoutQuarantineDuration)
			s.ConsecutiveTimeouts = 0
		}

	case FailureStreamError:
		s.StreamErrors++
		s.ConsecutiveTimeouts = 0
	}
}

// IsQuarantined checks whether a peer is currently serving a quarantine period at the given point in time.
func (rt *PeerReputationTracker) IsQuarantined(peerID peer.ID, at time.Time) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	s, exists := rt.stats[peerID]
	if !exists {
		return false
	}
	return at.Before(s.QuarantinedUntil)
}

// IsQuarantinedNow checks whether a peer is currently quarantined at the current moment.
func (rt *PeerReputationTracker) IsQuarantinedNow(peerID peer.ID) bool {
	return rt.IsQuarantined(peerID, time.Now())
}

// Score computes the current reputation score of a peer:
// Score(p) = 100 + min(50, 2*BlocksVerified) - 15*StreamErrors - 100*HashMismatchCount - (AvgLatencyMs / 50)
func (rt *PeerReputationTracker) Score(peerID peer.ID) float64 {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	s, exists := rt.stats[peerID]
	if !exists {
		return BaseReputationScore
	}

	bonus := math.Min(MaxBlockVerificationBonus, 2.0*float64(s.BlocksVerified))
	penaltyErrors := 15.0 * float64(s.StreamErrors)
	penaltyMismatches := 100.0 * float64(s.HashMismatchCount)
	penaltyLatency := s.AvgLatencyMs / 50.0

	return BaseReputationScore + bonus - penaltyErrors - penaltyMismatches - penaltyLatency
}

// GetStats returns a copy of the statistics recorded for the specified peer.
func (rt *PeerReputationTracker) GetStats(peerID peer.ID) (PeerTransferStats, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	s, exists := rt.stats[peerID]
	if !exists {
		return PeerTransferStats{PeerID: peerID}, false
	}
	return *s, true
}

// AllStats returns a snapshot of metrics for all tracked peers.
func (rt *PeerReputationTracker) AllStats() []PeerTransferStats {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	result := make([]PeerTransferStats, 0, len(rt.stats))
	for _, s := range rt.stats {
		result = append(result, *s)
	}
	return result
}

// ResetQuarantine clears the active quarantine duration and consecutive timeouts for a peer.
func (rt *PeerReputationTracker) ResetQuarantine(peerID peer.ID) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	if s, exists := rt.stats[peerID]; exists {
		s.QuarantinedUntil = time.Time{}
		s.ConsecutiveTimeouts = 0
	}
}

func (rt *PeerReputationTracker) getOrCreateLocked(peerID peer.ID) *PeerTransferStats {
	s, exists := rt.stats[peerID]
	if !exists {
		s = &PeerTransferStats{
			PeerID:   peerID,
			LastSeen: time.Now(),
		}
		rt.stats[peerID] = s
	}
	return s
}
