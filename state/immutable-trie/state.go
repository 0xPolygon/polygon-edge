package trie

import (
	lru "github.com/hashicorp/golang-lru"
	"github.com/umbracle/minimal/state"
	"github.com/umbracle/minimal/types"
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

func (s *State) addState(root types.Hash, t *Trie) {
	s.cache.Add(root, t)
}

func (s *State) NewSnapshot() state.Snapshot {
	t, _ := s.newTrieAtImpl(types.Hash{})
	t.state = s
	return t
}

func (s *State) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
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

func (s *State) newTrieAtImpl(root types.Hash) (*Trie, error) {
	if (root == types.Hash{}) {
		return NewTrie(), nil
	}
	return NewTrieAt(s.storage, root)
}

func (s *State) SetCode(hash types.Hash, code []byte) {
	s.storage.SetCode(hash, code)
}

func (s *State) GetCode(hash types.Hash) ([]byte, bool) {
	return s.storage.GetCode(hash)
}

func (s *State) Storage() Storage {
	return s.storage
}
