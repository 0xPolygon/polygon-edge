package statetransition

import (
	"bytes"
	"math/big"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	iradix "github.com/hashicorp/go-immutable-radix"
)

// StateObject is the internal representation of the account
type StateObject struct {
	Account   *types.Account
	Code      []byte
	Suicide   bool
	Deleted   bool
	DirtyCode bool
	Txn       *iradix.Txn

	// withFakeStorage signals whether the state object
	// is using the override full state
	withFakeStorage bool
}

func (s *StateObject) Empty() bool {
	return s.Account.Nonce == 0 && s.Account.Balance.Sign() == 0 && bytes.Equal(s.Account.CodeHash, emptyCodeHash)
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
	ss.withFakeStorage = s.withFakeStorage

	if s.Txn != nil {
		ss.Txn = s.Txn.CommitOnly().Txn()
	}

	return ss
}

// Object is the serialization of the radix object (can be merged to StateObject?).
type Object struct {
	Address  types.Address
	CodeHash types.Hash
	Balance  *big.Int
	Root     types.Hash
	Nonce    uint64
	Deleted  bool

	//nolint:godox
	// TODO: Move this to executor (to be fixed in EVM-527)
	DirtyCode bool
	Code      []byte

	Storage []*StorageObject
}

// StorageObject is an entry in the storage
type StorageObject struct {
	Deleted bool
	Key     []byte
	Val     []byte
}

var emptyCodeHash = crypto.Keccak256(nil)

var emptyCodeHashTwo = types.BytesToHash(crypto.Keccak256(nil))
