package shared

import (
	"bytes"

	iradix "github.com/hashicorp/go-immutable-radix"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

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
