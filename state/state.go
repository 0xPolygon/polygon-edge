package state

import (
	"bytes"
	"fmt"
	"math/big"

	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/minimal/rlp"
	"github.com/umbracle/minimal/crypto"
	"github.com/umbracle/minimal/types"
)

type State interface {
	NewSnapshotAt(types.Hash) (Snapshot, error)
	NewSnapshot() Snapshot
	GetCode(hash types.Hash) ([]byte, bool)
}

type Snapshot interface {
	Get(k []byte) ([]byte, bool)
	Commit(x *iradix.Tree) (Snapshot, []byte)
}

// account trie
type accountTrie interface {
	Get(k []byte) ([]byte, bool)
}

// Account is the account reference in the ethereum state
type Account struct {
	Nonce    uint64
	Balance  *big.Int
	Root     types.Hash
	CodeHash []byte
	Trie     accountTrie `rlp:"-"`
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
	aa.Trie = a.Trie

	return aa
}

var emptyCodeHash = crypto.Keccak256(nil)

// StateObject is the internal representation of the account
type StateObject struct {
	Account   *Account
	Code      []byte
	Suicide   bool
	Deleted   bool
	DirtyCode bool
	Txn       *iradix.Txn
}

func (s *StateObject) Empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, emptyCodeHash)
}

func (s *StateObject) GetCommitedState(hash types.Hash) types.Hash {
	val, ok := s.Account.Trie.Get(hash.Bytes())
	if !ok {
		return types.Hash{}
	}

	i := rlp.NewIterator(val)

	content, err := i.Bytes()
	if err != nil {
		return types.Hash{}
	}

	return types.BytesToHash(content)
}

// Copy makes a copy of the state object
func (s *StateObject) Copy() *StateObject {
	ss := new(StateObject)

	// copy account
	ss.Account = s.Account.Copy()

	ss.Suicide = s.Suicide
	ss.Deleted = s.Deleted
	ss.DirtyCode = s.DirtyCode
	ss.Code = s.Code

	if s.Txn != nil {
		ss.Txn = s.Txn.CommitOnly().Txn()
	}

	return ss
}
