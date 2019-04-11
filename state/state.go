package state

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	trie "github.com/umbracle/minimal/state/immutable-trie"
)

type State struct {
	state *trie.State
}

func NewState(state *trie.State) *State {
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
	tt    *trie.Trie
}

func (s *Snapshot) Txn() *Txn {
	return newTxn(s.state, s)
}

func (s *Snapshot) Get(k []byte) ([]byte, bool) {
	return s.tt.Get(k)
}

/*
// State is the ethereum state reference
type State struct {
	root    *trie.Trie
	storage trie.Storage
}

// NewState creates a new state
func NewState() *State {
	return &State{
		root: trie.NewTrie(),
	}
}

// NewStateAt returns the trie at a given hash. NOTE: this forgets the code completely
func NewStateAt(storage trie.Storage, root common.Hash) (*State, error) {
	t, err := trie.NewTrieAt(storage, root)
	if err != nil {
		return nil, err
	}

	s := &State{
		root:    t,
		storage: storage,
	}
	return s, nil
}

func (s *State) SetStorage(storage trie.Storage) {
	s.storage = storage
}

func (s *State) GetRoot() *trie.Trie {
	return s.getRoot()
}

// getRoot is used to do an atomic load of the root pointer
func (s *State) getRoot() *trie.Trie {
	return s.root
}

// Txn creates a Txn for the state
func (s *State) Txn() *Txn {
	return newTxn(s)
}

func (s *State) SetCode(hash common.Hash, code []byte) {
	s.storage.SetCode(hash, code)
}

func (s *State) GetCode(hash common.Hash) ([]byte, bool) {
	return s.storage.GetCode(hash)
}
*/

// Account is the account reference in the ethereum state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     common.Hash
	CodeHash []byte
	trie     *trie.Trie
}

func (a *Account) Trie() *trie.Trie {
	return a.trie
}

func (a *Account) String() string {
	return fmt.Sprintf("%d %s", a.Nonce, a.Balance.String())
}

func (a *Account) Copy() *Account {
	aa := new(Account)

	aa.Balance = big.NewInt(1).SetBytes(a.Balance.Bytes())
	aa.Nonce = a.Nonce
	aa.CodeHash = a.CodeHash
	aa.Root = a.Root
	aa.trie = a.trie

	return aa
}
