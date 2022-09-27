package itrie

import (
	"errors"
	"fmt"

	lru "github.com/hashicorp/golang-lru"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

type State struct {
	storage Storage
	cache   *lru.Cache
}

var _ state.State = (*State)(nil)

func NewState(storage Storage) *State {
	cache, _ := lru.New(128)

	s := &State{
		storage: storage,
		cache:   cache,
	}

	return s
}

func (s *State) SetCode(hash types.Hash, code []byte) {
	s.storage.SetCode(hash, code)
}

func (s *State) GetCode(hash types.Hash) ([]byte, bool) {
	return s.storage.GetCode(hash)
}

func (s *State) NewTrieAt(root types.Hash) (*Trie, error) {
	if root == types.EmptyRootHash {
		// empty state
		trie := NewTrie()
		trie.state = s
		trie.storage = s.storage
		return trie, nil
	}

	tt, ok := s.cache.Get(root)
	if ok {
		trie, ok := tt.(*Trie)

		if !ok {
			return nil, errors.New("invalid type assertion")
		}

		trie.state = s // update state of trie object in snapshot

		return trie, nil
	}

	n, ok, err := GetNode(root.Bytes(), s.storage)

	if err != nil {
		return nil, err
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

func (s *State) NewSnapshotAt(root types.Hash) (state.Snapshot, error) {
	accountTrie, err := s.NewTrieAt(root)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		accountTrie: accountTrie,
		state:       s,
	}
	return snap, nil
}

func (s *State) NewSnapshot() state.Snapshot {
	snap, _ := s.NewSnapshotAt(types.EmptyRootHash)
	return snap
}

func (s *State) Commit(objs []*state.Object) (state.Snapshot, []byte) {
	trie, err := s.NewTrieAt(types.EmptyRootHash)
	if err != nil {
		panic(err)
	}

	accountTrie, root := trie.Commit(objs)
	snap := &Snapshot{
		accountTrie: accountTrie,
		state:       s,
	}
	return snap, root
}

type Snapshot struct {
	accountTrie *Trie
	state       *State
}

var _ state.Snapshot = (*Snapshot)(nil)

func (snap *Snapshot) GetStorage(root types.Hash, key types.Hash) types.Hash {
	trie, err := snap.state.NewTrieAt(root)
	if err != nil {
		// TODO: log
		return types.Hash{}
	}

	keyIndex := crypto.Keccak256(key.Bytes())
	data, found := trie.Get(keyIndex)
	if !found {
		// TODO: log
		return types.Hash{}
	}

	return types.BytesToHash(data)
}

func (snap *Snapshot) GetAccount(addr types.Address) (*state.Account, error) {
	key := crypto.Keccak256(addr.Bytes())
	data, found := snap.accountTrie.Get(key)
	if !found {
		return nil, errors.New("address does not exist")
	}

	account := &state.Account{}
	account.UnmarshalRlp(data)
	return account, nil
}

func (snap Snapshot) GetCode(hash types.Hash) ([]byte, bool) {
	return snap.state.storage.GetCode(hash)
}
