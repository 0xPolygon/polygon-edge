package core

import (
	"io"
	"math/big"
	"sync"

	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// newRoundState creates a new roundState instance with the given view and validatorSet
// lockedHash and preprepare are for round change when lock exists,
// we need to keep a reference of preprepare in order to propose locked proposal when there is a lock and itself is the proposer
func newRoundState(view *ibft.View, validatorSet ibft.ValidatorSet, lockedHash types.Hash, preprepare *ibft.Preprepare, pendingRequest *ibft.Request, hasBadProposal func(hash types.Hash) bool) *roundState {
	return &roundState{
		round:          view.Round,
		sequence:       view.Sequence,
		preprepare:     preprepare,
		prepares:       newMessageSet(validatorSet),
		commits:        newMessageSet(validatorSet),
		lockedHash:     lockedHash,
		mu:             new(sync.RWMutex),
		pendingRequest: pendingRequest,
		hasBadProposal: hasBadProposal,
	}
}

// roundState stores the consensus state
type roundState struct {
	round          *big.Int         // uint64
	sequence       *big.Int         // uint64
	preprepare     *ibft.Preprepare // or maybe the proposal is here... not in pending
	prepares       *messageSet
	commits        *messageSet
	lockedHash     types.Hash
	pendingRequest *ibft.Request // the proposal of the round maybe?

	mu             *sync.RWMutex
	hasBadProposal func(hash types.Hash) bool
}

func (s *roundState) GetPrepareOrCommitSize() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := s.prepares.Size() + s.commits.Size()

	// find duplicate one
	for _, m := range s.prepares.Values() {
		if s.commits.Get(m.Address) != nil {
			result--
		}
	}
	return result
}

func (s *roundState) Subject() *ibft.Subject {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preprepare == nil {
		return nil
	}

	return &ibft.Subject{
		View: &ibft.View{
			Round:    new(big.Int).Set(s.round),
			Sequence: new(big.Int).Set(s.sequence),
		},
		Digest: s.preprepare.Proposal.Hash(),
	}
}

func (s *roundState) SetPreprepare(preprepare *ibft.Preprepare) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.preprepare = preprepare
}

func (s *roundState) Proposal() ibft.Proposal {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.preprepare != nil {
		return s.preprepare.Proposal
	}

	return nil
}

func (s *roundState) SetRound(r *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.round = new(big.Int).Set(r)
}

func (s *roundState) Round() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.round
}

func (s *roundState) SetSequence(seq *big.Int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sequence = seq
}

func (s *roundState) Sequence() *big.Int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sequence
}

func (s *roundState) LockHash() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.preprepare != nil {
		s.lockedHash = s.preprepare.Proposal.Hash()
	}
}

func (s *roundState) UnlockHash() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.lockedHash = types.Hash{}
}

func (s *roundState) IsHashLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if types.EmptyHash(s.lockedHash) {
		return false
	}
	return !s.hasBadProposal(s.GetLockedHash())
}

func (s *roundState) GetLockedHash() types.Hash {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lockedHash
}

// The DecodeRLP method should read one value from the given
// Stream. It is not forbidden to read less or more, but it might
// be confusing.
func (s *roundState) DecodeRLP(stream *rlp.Stream) error {
	var ss struct {
		Round          *big.Int
		Sequence       *big.Int
		Preprepare     *ibft.Preprepare
		Prepares       *messageSet
		Commits        *messageSet
		lockedHash     types.Hash
		pendingRequest *ibft.Request
	}

	if err := stream.Decode(&ss); err != nil {
		return err
	}
	s.round = ss.Round
	s.sequence = ss.Sequence
	s.preprepare = ss.Preprepare
	s.prepares = ss.Prepares
	s.commits = ss.Commits
	s.lockedHash = ss.lockedHash
	s.pendingRequest = ss.pendingRequest
	s.mu = new(sync.RWMutex)

	return nil
}

// EncodeRLP should write the RLP encoding of its receiver to w.
// If the implementation is a pointer method, it may also be
// called for nil pointers.
//
// Implementations should generate valid RLP. The data written is
// not verified at the moment, but a future version might. It is
// recommended to write only a single value but writing multiple
// values or no value at all is also permitted.
func (s *roundState) EncodeRLP(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return rlp.Encode(w, []interface{}{
		s.round,
		s.sequence,
		s.preprepare,
		s.prepares,
		s.commits,
		s.lockedHash,
		s.pendingRequest,
	})
}
