package trie

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/umbracle/minimal/state"

	"github.com/ethereum/go-ethereum/common"
)

type State struct {
	storage Storage
	cache   *lru.Cache
}

func NewState(storage Storage) *State {
	cache, _ := lru.New(128)

	s := &State{
		storage: storage,
		cache:   cache,
	}
	return s
}

func (s *State) addState(root common.Hash, t *Trie) {
	s.cache.Add(root, t)
}

func (s *State) NewSnapshot() state.Snapshot {
	t, _ := s.newTrieAtImpl(common.Hash{})
	t.state = s
	return t
}

func (s *State) NewSnapshotAt(root common.Hash) (state.Snapshot, error) {
	// Check locally.
	tt, ok := s.cache.Get(root)
	if ok {
		return tt.(*Trie), nil
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
