package state

import (
	"fmt"
	"math/big"
	"sync/atomic"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/umbracle/minimal/state/trie"
)

// State is the ethereum state reference
type State struct {
	root    unsafe.Pointer
	code    map[string][]byte
	storage trie.Storage
}

// NewState creates a new state
func NewState() *State {
	return &State{
		root: unsafe.Pointer(trie.NewTrie()),
		code: map[string][]byte{},
	}
}

// NewStateAt returns the trie at a given hash. NOTE: this forgets the code completely
func NewStateAt(storage trie.Storage, root common.Hash) (*State, error) {
	t, err := trie.NewTrieAt(storage, root)
	if err != nil {
		return nil, err
	}

	s := &State{
		root:    unsafe.Pointer(t),
		code:    map[string][]byte{},
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
	root := (*trie.Trie)(atomic.LoadPointer(&s.root))
	return root
}

// Txn creates a Txn for the state
func (s *State) Txn() *Txn {
	return newTxn(s)
}

func (s *State) SetCode(hash common.Hash, code []byte) {
	s.code[hash.String()] = code
}

func (s *State) GetCode(hash common.Hash) ([]byte, bool) {
	code, ok := s.code[hash.String()]
	return code, ok
}

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
	return fmt.Sprintf("%d %d", a.Nonce, a.Balance.Uint64())
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
