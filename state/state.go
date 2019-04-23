package state

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	iradix "github.com/hashicorp/go-immutable-radix"
)

type State interface {
	NewSnapshotAt(common.Hash) (Snapshot, error)
	NewSnapshot() Snapshot
	GetCode(hash common.Hash) ([]byte, bool)
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
	Root     common.Hash
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

func (s *StateObject) GetCommitedState(hash common.Hash) common.Hash {
	val, ok := s.Account.Trie.Get(hash.Bytes())
	if !ok {
		return common.Hash{}
	}
	_, content, _, err := rlp.Split(val)
	if err != nil {
		return common.Hash{}
	}
	return common.BytesToHash(content)
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
