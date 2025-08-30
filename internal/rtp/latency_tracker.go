package rtp

import (
	"sync"
	"time"
)

// LatencyTracker tracks round-trip latency for RTP packets using sequence numbers
type LatencyTracker struct {
	sentTimes map[uint16]time.Time // seq -> send_time
	mu        sync.RWMutex
}

// NewLatencyTracker creates a new LatencyTracker
func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		sentTimes: make(map[uint16]time.Time),
	}
}

// RecordSent records the time when an RTP packet is sent to the echo server
func (t *LatencyTracker) RecordSent(seq uint16, sendTime time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.sentTimes[seq] = sendTime

	// Clean up old entries to prevent memory leaks
	// Remove entries older than 3 seconds (more aggressive cleanup)
	cutoff := time.Now().Add(-3 * time.Second)
	for seq, sentTime := range t.sentTimes {
		if sentTime.Before(cutoff) {
			delete(t.sentTimes, seq)
		}
	}
}

// GetLatency calculates the latency for a received packet by sequence number
func (t *LatencyTracker) GetLatency(seq uint16) (time.Duration, bool) {
	// First, try to read with RLock for better concurrency
	t.mu.RLock()
	if sendTime, exists := t.sentTimes[seq]; exists {
		t.mu.RUnlock()

		// Now take write lock to remove the entry
		t.mu.Lock()
		defer t.mu.Unlock()

		// Double-check that the entry still exists (in case it was removed by another goroutine)
		if sendTime, exists = t.sentTimes[seq]; exists {
			latency := time.Since(sendTime)
			delete(t.sentTimes, seq) // Remove entry after use
			return latency, true
		}

		// Entry was removed by another goroutine
		return 0, false
	}
	t.mu.RUnlock()

	// Entry not found
	return 0, false
}
