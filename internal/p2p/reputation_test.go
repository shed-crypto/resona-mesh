package p2p

import (
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

func TestPeerReputationTracker_BaseScore(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-test-1")

	score := tracker.Score(peerID)
	if score != BaseReputationScore {
		t.Fatalf("expected base score %f, got %f", BaseReputationScore, score)
	}

	if tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("untracked peer should not be quarantined")
	}
}

func TestPeerReputationTracker_SuccessAndLatencyEMA(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-test-2")

	// First success: latency 100ms
	tracker.RecordSuccess(peerID, 100*time.Millisecond, 256*1024)
	stats, ok := tracker.GetStats(peerID)
	if !ok {
		t.Fatalf("expected peer stats to exist")
	}
	if stats.BlocksVerified != 1 {
		t.Fatalf("expected 1 verified block, got %d", stats.BlocksVerified)
	}
	if stats.AvgLatencyMs != 100.0 {
		t.Fatalf("expected 100ms initial latency, got %f", stats.AvgLatencyMs)
	}

	// Score = 100 + 2 - 0 - 0 - (100 / 50) = 100
	expectedScore := 100.0 + 2.0 - 0.0 - 0.0 - (100.0 / 50.0)
	if tracker.Score(peerID) != expectedScore {
		t.Fatalf("expected score %f, got %f", expectedScore, tracker.Score(peerID))
	}

	// Second success: latency 50ms
	// EMA: 0.8 * 100 + 0.2 * 50 = 80 + 10 = 90ms
	tracker.RecordSuccess(peerID, 50*time.Millisecond, 256*1024)
	stats, _ = tracker.GetStats(peerID)
	if stats.BlocksVerified != 2 {
		t.Fatalf("expected 2 verified blocks, got %d", stats.BlocksVerified)
	}
	if stats.AvgLatencyMs != 90.0 {
		t.Fatalf("expected 90ms EMA latency, got %f", stats.AvgLatencyMs)
	}

	// Verify cap on blocks verified bonus (max 50)
	for i := 0; i < 50; i++ {
		tracker.RecordSuccess(peerID, 50*time.Millisecond, 256*1024)
	}
	stats, _ = tracker.GetStats(peerID)
	if stats.BlocksVerified != 52 {
		t.Fatalf("expected 52 verified blocks, got %d", stats.BlocksVerified)
	}
	score := tracker.Score(peerID)
	// Bonus capped at 50, latency approx 50ms (so -1 penalty)
	// Score should be approximately 100 + 50 - 1 = 149
	if score < 148.0 || score > 150.0 {
		t.Fatalf("expected capped score near 149, got %f", score)
	}
}

func TestPeerReputationTracker_HashMismatchAndQuarantineDoubling(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-malicious-1")

	// 1. First hash mismatch
	now := time.Now()
	tracker.RecordFailure(peerID, FailureHashMismatch)

	stats, ok := tracker.GetStats(peerID)
	if !ok || stats.HashMismatchCount != 1 {
		t.Fatalf("expected 1 hash mismatch, got %d", stats.HashMismatchCount)
	}
	if !tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("peer should be quarantined after hash mismatch")
	}

	// Quarantine should be approximately 10 minutes in the future
	expectedQuarantine := now.Add(DefaultQuarantineBaseDuration)
	if stats.QuarantinedUntil.Before(expectedQuarantine.Add(-2*time.Second)) ||
		stats.QuarantinedUntil.After(expectedQuarantine.Add(2*time.Second)) {
		t.Fatalf("expected ~10m quarantine, got %v", stats.QuarantinedUntil)
	}

	// Score = 100 + 0 - 0 - 100 - 0 = 0
	if tracker.Score(peerID) != 0.0 {
		t.Fatalf("expected score 0 after mismatch, got %f", tracker.Score(peerID))
	}

	// 2. Second hash mismatch (repeat offense) -> 20 minutes
	tracker.RecordFailure(peerID, FailureHashMismatch)
	stats, _ = tracker.GetStats(peerID)
	if stats.QuarantineCount != 2 {
		t.Fatalf("expected quarantine count 2, got %d", stats.QuarantineCount)
	}
	expectedSecondQuarantine := time.Now().Add(20 * time.Minute)
	if stats.QuarantinedUntil.Before(expectedSecondQuarantine.Add(-2*time.Second)) ||
		stats.QuarantinedUntil.After(expectedSecondQuarantine.Add(2*time.Second)) {
		t.Fatalf("expected ~20m doubled quarantine, got %v", stats.QuarantinedUntil)
	}

	// 3. Reset quarantine
	tracker.ResetQuarantine(peerID)
	if tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("peer should not be quarantined after ResetQuarantine")
	}
}

func TestPeerReputationTracker_TimeoutQuarantine(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-timeout-1")

	// 1st timeout: not quarantined
	tracker.RecordFailure(peerID, FailureTimeout)
	if tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("should not be quarantined on 1st timeout")
	}

	// 2nd timeout: not quarantined
	tracker.RecordFailure(peerID, FailureTimeout)
	if tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("should not be quarantined on 2nd timeout")
	}

	// 3rd timeout: triggers 1-minute quarantine
	now := time.Now()
	tracker.RecordFailure(peerID, FailureTimeout)
	if !tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("should be quarantined on 3rd consecutive timeout")
	}

	stats, _ := tracker.GetStats(peerID)
	expectedQuarantine := now.Add(TimeoutQuarantineDuration)
	if stats.QuarantinedUntil.Before(expectedQuarantine.Add(-2*time.Second)) ||
		stats.QuarantinedUntil.After(expectedQuarantine.Add(2*time.Second)) {
		t.Fatalf("expected ~1m quarantine, got %v", stats.QuarantinedUntil)
	}

	// Check IsQuarantined with past and future timestamps
	if !tracker.IsQuarantined(peerID, now.Add(30*time.Second)) {
		t.Fatalf("should be quarantined after 30 seconds")
	}
	if tracker.IsQuarantined(peerID, now.Add(2*time.Minute)) {
		t.Fatalf("should not be quarantined after 2 minutes")
	}
}

func TestPeerReputationTracker_SuccessResetsConsecutiveTimeouts(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-flaky-1")

	// 2 timeouts
	tracker.RecordFailure(peerID, FailureTimeout)
	tracker.RecordFailure(peerID, FailureTimeout)

	// 1 success
	tracker.RecordSuccess(peerID, 50*time.Millisecond, 1024)

	stats, _ := tracker.GetStats(peerID)
	if stats.ConsecutiveTimeouts != 0 {
		t.Fatalf("expected consecutive timeouts to reset to 0, got %d", stats.ConsecutiveTimeouts)
	}

	// Another 2 timeouts shouldn't quarantine because counter was reset
	tracker.RecordFailure(peerID, FailureTimeout)
	tracker.RecordFailure(peerID, FailureTimeout)
	if tracker.IsQuarantinedNow(peerID) {
		t.Fatalf("should not be quarantined when timeouts are interrupted by success")
	}
}

func TestPeerReputationTracker_ConcurrentSafety(t *testing.T) {
	tracker := NewPeerReputationTracker()
	peerID := peer.ID("peer-concurrent")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			tracker.RecordSuccess(peerID, 20*time.Millisecond, 256)
		}()
		go func() {
			defer wg.Done()
			tracker.RecordFailure(peerID, FailureStreamError)
		}()
		go func() {
			defer wg.Done()
			_ = tracker.Score(peerID)
			_ = tracker.IsQuarantinedNow(peerID)
		}()
	}
	wg.Wait()

	all := tracker.AllStats()
	if len(all) != 1 {
		t.Fatalf("expected 1 tracked peer, got %d", len(all))
	}
}

