package stats

import (
	"sync"
	"time"
)

type Stats struct {
	lock *sync.Mutex

	round    uint64
	sequence uint64

	msgCount       map[string]uint64
	msgVotingPower map[string]uint64
	stateDuration  map[string]time.Duration
}

func NewStats() *Stats {
	return &Stats{
		lock:           &sync.Mutex{},
		msgCount:       make(map[string]uint64),
		msgVotingPower: make(map[string]uint64),
		stateDuration:  make(map[string]time.Duration),
	}
}

func (s *Stats) SetView(sequence uint64, round uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.sequence = sequence
	s.round = round
}

func (s *Stats) IncrMsgCount(msgType string, votingPower uint64) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.msgCount[msgType]++
	s.msgVotingPower[msgType] += votingPower
}

func (s *Stats) StateDuration(state string, t time.Time) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.stateDuration[state] = time.Since(t)
}

func (s *Stats) Snapshot() Stats {
	// Allocate a new stats struct
	stats := NewStats()
	s.lock.Lock()
	defer s.lock.Unlock()

	stats.round = s.round
	stats.sequence = s.sequence

	for msgType, count := range s.msgCount {
		stats.msgCount[msgType] = count
	}

	for msgType, votingPower := range s.msgVotingPower {
		stats.msgVotingPower[msgType] = votingPower
	}

	for msgType, duration := range s.stateDuration {
		stats.stateDuration[msgType] = duration
	}

	return *stats
}

func (s *Stats) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.msgCount = make(map[string]uint64)
	s.msgVotingPower = make(map[string]uint64)
	s.stateDuration = make(map[string]time.Duration)
}
