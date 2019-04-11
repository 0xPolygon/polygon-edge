package trie

import (
	lru "github.com/hashicorp/golang-lru"

	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/state/shared"
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

func (s *State) NewTrie() shared.Trie {
	t, _ := s.newTrieAtImpl(common.Hash{})
	t.state = s
	return t
}

func (s *State) NewTrieAt(root common.Hash) (shared.Trie, error) {
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
