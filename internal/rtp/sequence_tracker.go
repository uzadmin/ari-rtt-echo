package rtp

import (
	"sync"
)

// SequenceTracker tracks sequence numbers for packet loss detection
type SequenceTracker struct {
	lastOutgoingSeq uint16
	lastIncomingSeq uint16
	outgoingCount   uint32
	incomingCount   uint32
	droppedCount    uint32
	mu              sync.RWMutex
}

// NewSequenceTracker creates a new SequenceTracker
func NewSequenceTracker() *SequenceTracker {
	return &SequenceTracker{}
}

// TrackOutgoing tracks outgoing packet sequence numbers
func (s *SequenceTracker) TrackOutgoing(seq uint16) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastOutgoingSeq = seq
	s.outgoingCount++
}

// TrackIncoming tracks incoming packet sequence numbers and detects drops
func (s *SequenceTracker) TrackIncoming(seq uint16) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.incomingCount++

	// Detect packet drops by checking sequence number gaps
	if s.lastIncomingSeq != 0 {
		expectedSeq := s.lastIncomingSeq + 1
		if seq != expectedSeq {
			// Handle sequence number wraparound
			var dropped uint32
			if seq < s.lastIncomingSeq {
				// Wraparound occurred
				dropped = (uint32(0xFFFF) - uint32(s.lastIncomingSeq)) + uint32(seq) - 1
			} else {
				// Normal gap
				dropped = uint32(seq) - uint32(s.lastIncomingSeq) - 1
			}
			s.droppedCount += dropped
			return dropped // Return only the newly detected drops
		}
	}

	s.lastIncomingSeq = seq
	return 0 // No drops detected
}

// GetStats returns tracking statistics
func (s *SequenceTracker) GetStats() (outgoing, incoming, dropped uint32) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.outgoingCount, s.incomingCount, s.droppedCount
}
