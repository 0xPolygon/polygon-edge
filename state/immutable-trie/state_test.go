package trie

import (
	"math/big"
	"testing"

	"github.com/umbracle/minimal/state"
)

func buildPrestate(preState state.PreStates) (state.State, state.Snapshot) {
	s := NewState(NewMemoryStorage())
	snap := s.NewSnapshot()

	txn := state.NewTxn(s, snap)
	for i, j := range preState {
		txn.SetNonce(i, j.Nonce)
		txn.SetBalance(i, big.NewInt(int64(j.Balance)))
		for k, v := range j.State {
			txn.SetState(i, k, v)
		}
	}
	snap, _ = txn.Commit(false)
	return s, snap
}

func TestState(t *testing.T) {
	state.TestState(t, buildPrestate)
}
