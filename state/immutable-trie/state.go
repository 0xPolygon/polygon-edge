package trie

import (
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

type State struct {
	storage       Storage
	snapshots     map[common.Hash]*Trie
	snapshotsLock sync.Mutex
}

func NewState(storage Storage) *State {
	s := &State{
		storage:       storage,
		snapshots:     map[common.Hash]*Trie{},
		snapshotsLock: sync.Mutex{},
	}
	return s
}

func (s *State) addState(root common.Hash, t *Trie) {
	s.snapshotsLock.Lock()
	defer s.snapshotsLock.Unlock()

	s.snapshots[root] = t
}

func (s *State) NewTrieAt(root common.Hash) (*Trie, error) {
	// Check locally.
	s.snapshotsLock.Lock()
	t, ok := s.snapshots[root]
	s.snapshotsLock.Unlock()

	if ok {
		return t, nil
	}

	t, err := s.newTrieAtImpl(root)
	if err != nil {
		return nil, err
	}
	t.state = s
	return t, nil
}

func (s *State) newTrieAtImpl(root common.Hash) (*Trie, error) {
	if (root == common.Hash{}) {
		return NewTrie(), nil
	}
	return NewTrieAt(s.storage, root)
}

func (s *State) SetCode(hash common.Hash, code []byte) {
	s.storage.SetCode(hash, code)
}

func (s *State) GetCode(hash common.Hash) ([]byte, bool) {
	return s.storage.GetCode(hash)
}

func (s *State) Storage() Storage {
	return s.storage
}
