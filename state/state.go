package state

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/state/shared"
)

// TODO, remove this structures since they are redundant

type State struct {
	state shared.State
}

func NewState(state shared.State) *State {
	return &State{state: state}
}

func (s *State) NewSnapshot(root common.Hash) (*Snapshot, bool) {
	t, err := s.state.NewTrieAt(root)
	if err != nil {
		panic(err)
	}
	return &Snapshot{state: s, tt: t}, true
}

type Snapshot struct {
	state *State
	tt    shared.Trie
}

func (s *Snapshot) Txn() *Txn {
	return newTxn(s.state, s)
}

func (s *Snapshot) Get(k []byte) ([]byte, bool) {
	return s.tt.Get(k)
}
