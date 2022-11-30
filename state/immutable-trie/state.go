package itrie

import (
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru"

	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

type State struct {
	storage Storage
	cache   *lru.Cache
	m       *sync.RWMutex // use a pointer to prevent copying of the mutex in setState/setForTrie
}

func NewState(storage Storage) *State {
	cache, _ := lru.New(128)

	s := &State{
		storage: storage,
		cache:   cache,
		m:       new(sync.RWMutex),
	}

	return s
}

func (s *State) NewSnapshot() state.Snapshot {
	return &Snapshot{state: s, trie: s.newTrie()}
}

func (s *State) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	t, err := s.newTrieAt(root)
	if err != nil {
		return nil, err
	}

	return &Snapshot{state: s, trie: t}, nil
}

func (s *State) newTrie() *Trie {
	t := NewTrie()
	t.state = s
	t.storage = s.storage

	return t
}

func (s *State) SetCode(hash types.Hash, code []byte) {
	s.storage.SetCode(hash, code)
}

func (s *State) GetCode(hash types.Hash) ([]byte, bool) {
	return s.storage.GetCode(hash)
}

func (s *State) newTrieAt(root types.Hash) (*Trie, error) {
	if root == types.EmptyRootHash {
		// empty state
		return s.newTrie(), nil
	}

	tt, ok := s.cache.Get(root)
	if ok {
		t, ok := tt.(*Trie)
		if !ok {
			return nil, fmt.Errorf("invalid type assertion on root: %s", root)
		}

		t.state.setState(s)

		trie, ok := tt.(*Trie)
		if !ok {
			return nil, fmt.Errorf("invalid type assertion on root: %s", root)
		}

		return trie, nil
	}

	n, ok, err := GetNode(root.Bytes(), s.storage)
	if err != nil {
		return nil, fmt.Errorf("failed to get storage root %s: %w", root, err)
	}

	if !ok {
		return nil, fmt.Errorf("state not found at hash %s", root)
	}

	t := &Trie{
		root:    n,
		state:   s,
		storage: s.storage,
	}

	return t, nil
}

func (s *State) AddState(root types.Hash, t *Trie) {
	s.cache.Add(root, t)
}

func (s *State) setState(s1 *State) {
	m := s.m
	m1 := s1.m

	m.Lock()

	m1.RLock()
	*s = *s1
	m1.RUnlock()

	m.Unlock()
}

func (s *State) setForTrie(t *Trie) {
	t.state.m.Lock()

	s.m.RLock()
	t.state = s
	s.m.RUnlock()

	t.state.m.Unlock()
}
