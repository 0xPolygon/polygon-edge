package core

import (
	"math/big"
	"sync"
	"testing"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
)

func newTestRoundState(view *ibft.View, validatorSet ibft.ValidatorSet) *roundState {
	return &roundState{
		round:      view.Round,
		sequence:   view.Sequence,
		Preprepare: newTestPreprepare(view),
		Prepares:   newMessageSet(validatorSet),
		Commits:    newMessageSet(validatorSet),
		mu:         new(sync.RWMutex),
		hasBadProposal: func(hash types.Hash) bool {
			return false
		},
	}
}

func TestLockHash(t *testing.T) {
	sys := NewTestSystemWithBackend(1, 0)
	rs := newTestRoundState(
		&ibft.View{
			Round:    big.NewInt(0),
			Sequence: big.NewInt(0),
		},
		sys.backends[0].peers,
	)
	if !types.EmptyHash(rs.GetLockedHash()) {
		t.Errorf("error mismatch: have %v, want empty", rs.GetLockedHash())
	}
	if rs.IsHashLocked() {
		t.Error("IsHashLocked should return false")
	}

	// Lock
	expected := rs.Proposal().Hash()
	rs.LockHash()
	if expected != rs.GetLockedHash() {
		t.Errorf("error mismatch: have %v, want %v", rs.GetLockedHash(), expected)
	}
	if !rs.IsHashLocked() {
		t.Error("IsHashLocked should return true")
	}

	// Unlock
	rs.UnlockHash()
	if !types.EmptyHash(rs.GetLockedHash()) {
		t.Errorf("error mismatch: have %v, want empty", rs.GetLockedHash())
	}
	if rs.IsHashLocked() {
		t.Error("IsHashLocked should return false")
	}
}
