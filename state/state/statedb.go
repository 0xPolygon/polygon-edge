package state

import (
	"math/big"
	"sync/atomic"
	"unsafe"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/hashicorp/go-immutable-radix"
)

var (
	// logIndex is the index of the logs in the trie
	logIndex = []byte{2}
)

func todo() {
	panic("TODO")
}

// Trie references the underlaying trie storage
type Trie interface {
	Get(k []byte) ([]byte, bool)
}

// StateDB is a reference of the state
type StateDB struct {
	root      unsafe.Pointer // *iradix.Tree underneath
	trie      Trie
	snapshots []*iradix.Tree
	txn       *iradix.Txn
}

// NewStateDB creates a new state reference
func NewStateDB(trie Trie) *StateDB {
	i := iradix.New()

	return &StateDB{
		trie:      trie,
		root:      unsafe.Pointer(i),
		snapshots: []*iradix.Tree{},
		txn:       i.Txn(),
	}
}

// IDEA, commit the value before you make the new snapshot? that should indeed mark a good transition
// We need a commit function that really commits the values in case there was no snapshot

func (s *StateDB) ForceCommit() {
	tree := s.txn.CommitOnly()
	atomic.StorePointer(&s.root, unsafe.Pointer(tree))
	s.txn = tree.Txn() // weird
}

func (s *StateDB) getRoot() *iradix.Tree {
	root := (*iradix.Tree)(atomic.LoadPointer(&s.root))
	return root
}

// Snapshot takes a snapshot at this point in time
func (s *StateDB) Snapshot() int {
	t := s.txn.CommitOnly()

	id := len(s.snapshots)
	s.snapshots = append(s.snapshots, t)
	return id
}

// RevertToSnapshot reverts to a given snapshot
func (s *StateDB) RevertToSnapshot(id int) {
	if id > len(s.snapshots) {
		panic("")
	}

	tree := s.snapshots[id]

	// TODO, remove the old snapshots
	atomic.StorePointer(&s.root, unsafe.Pointer(tree))
	s.txn = tree.Txn() // weird
}

// GetAccount returns an account
func (s *StateDB) GetAccount(addr common.Address) (*Account, bool) {
	val, exists := s.txn.Get(addr.Bytes())
	if exists {
		return val.(*stateObject).account, true
	}
	return nil, false
}

// stateObject is the internal representation of the account
type stateObject struct {
	account   *Account
	code      []byte
	suicide   bool
	dirtyCode bool
	storage   map[common.Hash]common.Hash
}

// Copy makes a copy of the state object
func (s *stateObject) Copy() *stateObject {
	ss := new(stateObject)

	// copy account
	aa := new(Account)
	aa.Balance = big.NewInt(1).SetBytes(s.account.Balance.Bytes())
	aa.Nonce = s.account.Nonce
	ss.account = aa

	// copy storage
	ss.storage = map[common.Hash]common.Hash{}
	for k, v := range s.storage {
		ss.storage[k] = v
	}

	ss.suicide = s.suicide
	ss.dirtyCode = s.dirtyCode
	ss.code = s.code

	return ss
}

func (s *StateDB) getStateObject(addr common.Address) (*stateObject, bool) {
	val, exists := s.txn.Get(addr.Bytes())
	if exists {
		return val.(*stateObject).Copy(), true
	}

	data, ok := s.trie.Get(addr.Bytes())
	if !ok {
		return nil, false
	}

	var account Account
	if err := rlp.DecodeBytes(data, &account); err != nil {
		panic(err)
	}

	stateObject := &stateObject{
		account: &account,
	}
	return stateObject, true
}

func (s *StateDB) upsertAccount(addr common.Address, create bool, f func(object *stateObject)) {
	object, exists := s.getStateObject(addr)
	if !exists && create {
		object = &stateObject{
			account: &Account{Balance: big.NewInt(0)},
		}
	}

	// run the callback to modify the account
	f(object)

	if object != nil {
		s.txn.Insert(addr.Bytes(), object)
	}
}

// AddBalance adds balance
func (s *StateDB) AddBalance(addr common.Address, balance *big.Int) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance.Add(object.account.Balance, balance)
	})
}

// SubBalance reduces the balance
func (s *StateDB) SubBalance(addr common.Address, balance *big.Int) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance.Sub(object.account.Balance, balance)
	})
}

// SetBalance sets the balance
func (s *StateDB) SetBalance(addr common.Address, balance *big.Int) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance = balance
	})
}

// GetBalance returns the balance of an address
func (s *StateDB) GetBalance(addr common.Address) *big.Int {
	object, exists := s.getStateObject(addr)
	if !exists {
		return big.NewInt(0)
	}
	return object.account.Balance
}

// AddLog adds a new log
func (s *StateDB) AddLog(log *types.Log) {
	var logs []*types.Log

	data, exists := s.txn.Get(logIndex)
	if !exists {
		logs = []*types.Log{}
	} else {
		logs = data.([]*types.Log)
	}

	logs = append(logs, log)
	s.txn.Insert(logIndex, logs)
}

// State

// SetState change the state of an address
func (s *StateDB) SetState(addr common.Address, key, value common.Hash) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.storage[key] = value
	})
}

// GetState returns the state of the address at a given hash
func (s *StateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	object, exists := s.getStateObject(addr)
	if !exists {
		return common.Hash{}
	}
	val, ok := object.storage[hash]
	if ok {
		return val
	}
	return common.Hash{}
}

// Nonce

// SetNonce reduces the balance
func (s *StateDB) SetNonce(addr common.Address, nonce uint64) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Nonce = nonce
	})
}

// GetNonce returns the nonce of an addr
func (s *StateDB) GetNonce(addr common.Address) uint64 {
	object, exists := s.getStateObject(addr)
	if !exists {
		return 0
	}
	return object.account.Nonce
}

// Code

// SetCode sets the code for an address
func (s *StateDB) SetCode(addr common.Address, code []byte) {
	s.upsertAccount(addr, true, func(object *stateObject) {
		object.account.CodeHash = crypto.Keccak256Hash(code).Bytes()
		object.dirtyCode = true
		object.code = code
	})
}

func (s *StateDB) GetCode(addr common.Address) []byte {
	object, exists := s.getStateObject(addr)
	if !exists {
		return nil
	}
	return object.code
}

func (s *StateDB) GetCodeSize(addr common.Address) int {
	object, exists := s.getStateObject(addr)
	if !exists {
		return 0
	}
	return len(object.code)
}

func (s *StateDB) GetCodeHash(addr common.Address) common.Hash {
	object, exists := s.getStateObject(addr)
	if !exists {
		return common.Hash{}
	}
	return common.BytesToHash(object.account.CodeHash)
}

// Suicide

// Suicide marks the given account as suicided
func (s *StateDB) Suicide(addr common.Address) bool {
	var suicided bool
	s.upsertAccount(addr, false, func(object *stateObject) {
		if object == nil || object.suicide {
			suicided = false
		} else {
			suicided = true
			object.suicide = true
			object.account.Balance = new(big.Int)
		}
	})
	return suicided
}

// HasSuicided returns true if the account suicided
func (s *StateDB) HasSuicided(addr common.Address) bool {
	object, exists := s.getStateObject(addr)
	return exists && object.suicide
}

// Refund
func (s *StateDB) AddRefund(gas uint64) {

}

func (s *StateDB) SubRefund(gas uint64) {

}

func (s *StateDB) Logs() []*types.Log {
	data, exists := s.txn.Get(logIndex)
	if !exists {
		return nil
	}
	return data.([]*types.Log)
}

func (s *StateDB) GetRefund() uint64 {
	return 0
}

func (s *StateDB) IntermediateRoot(bool) common.Hash {
	return common.Hash{}
}

// GetCommittedState returns the state of the address in the trie
func (s *StateDB) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	return common.Hash{}
}

func (s *StateDB) Exist(addr common.Address) bool {
	return false
}

func (s *StateDB) Empty(addr common.Address) bool {
	return false
}

func (s *StateDB) CreateAccount(addr common.Address) {
}
