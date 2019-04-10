package state

import (
	"bytes"
	"errors"
	"math/big"
	"unsafe"

	"github.com/ethereum/go-ethereum/rlp"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/minimal/state/trie"
)

var (
	ErrInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

var emptyCodeHash = crypto.Keccak256(nil)

var emptyStateHash = common.HexToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

var (
	// logIndex is the index of the logs in the trie
	logIndex = common.BytesToHash([]byte{2}).Bytes()

	// refundIndex is the index of the refund
	refundIndex = common.BytesToHash([]byte{3}).Bytes()
)

type GasPool interface {
	SubGas(uint64) error
	AddGas(uint64)
}

// Trie references the underlaying trie storage
type Trie interface {
	Get(k []byte) (interface{}, bool)
}

// Txn is a reference of the state
type Txn struct {
	state      *State
	snapshots  []*iradix.Tree
	txn        *iradix.Txn
	gas        uint64
	initialGas uint64
}

// newTxn creates a new state reference
func newTxn(state *State) *Txn {
	i := iradix.New()

	return &Txn{
		state:     state,
		snapshots: []*iradix.Tree{},
		txn:       i.Txn(),
	}
}

// gasUsed returns the amount of gas used up by the state transition.
func (txn *Txn) gasUsed() uint64 {
	return txn.initialGas - txn.gas
}

// Snapshot takes a snapshot at this point in time
func (txn *Txn) Snapshot() int {
	t := txn.txn.CommitOnly()

	id := len(txn.snapshots)
	txn.snapshots = append(txn.snapshots, t)

	// fmt.Printf("take snapshot ========> %d\n", id)

	return id
}

// RevertToSnapshot reverts to a given snapshot
func (txn *Txn) RevertToSnapshot(id int) {
	// fmt.Printf("revert to snapshot ======> %d\n", id)

	if id > len(txn.snapshots) {
		panic("")
	}

	tree := txn.snapshots[id]
	txn.txn = tree.Txn()
}

// GetAccount returns an account
func (txn *Txn) GetAccount(addr common.Address) (*Account, bool) {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil, false
	}
	return object.account, true
}

// stateObject is the internal representation of the account
type stateObject struct {
	account   *Account
	code      []byte
	suicide   bool
	deleted   bool
	dirtyCode bool
	txn       *iradix.Txn
}

func (s *stateObject) Empty() bool {
	return s.account.Nonce == 0 && s.account.Balance.Sign() == 0 && bytes.Equal(s.account.CodeHash, emptyCodeHash)
}

func (s *stateObject) GetCommitedState(hash common.Hash) common.Hash {
	val, ok := s.account.trie.Get(hash.Bytes())
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
func (s *stateObject) Copy() *stateObject {
	ss := new(stateObject)

	// copy account
	ss.account = s.account.Copy()

	ss.suicide = s.suicide
	ss.deleted = s.deleted
	ss.dirtyCode = s.dirtyCode
	ss.code = s.code

	if s.txn != nil {
		ss.txn = s.txn.CommitOnly().Txn()
	}

	return ss
}

func (txn *Txn) getStateObject(addr common.Address) (*stateObject, bool) {
	val, exists := txn.txn.Get(addr.Bytes())
	if exists {
		obj := val.(*stateObject)
		if obj.deleted {
			return nil, false
		}
		return obj.Copy(), true
	}

	// From the state we get the account object
	data, ok := txn.state.getRoot().Get(hashit(addr.Bytes()))
	if !ok {
		return nil, false
	}

	var account Account
	err := rlp.DecodeBytes(data, &account)
	if err != nil {
		return nil, false
	}

	// Load trie from memory if there is some state
	if account.Root == emptyStateHash {
		account.trie = trie.NewTrie()
	} else {
		// TODO, load from state that keeps a cache of tries
		account.trie, err = trie.NewTrieAt(txn.state.storage, account.Root)
		if err != nil {
			return nil, false
		}
	}

	obj := &stateObject{
		account: account.Copy(),
	}
	return obj, true
}

func (txn *Txn) upsertAccount(addr common.Address, create bool, f func(object *stateObject)) {
	object, exists := txn.getStateObject(addr)
	if !exists && create {
		object = &stateObject{
			account: &Account{
				Balance:  big.NewInt(0),
				trie:     trie.NewTrie(),
				CodeHash: emptyCodeHash,
				Root:     emptyStateHash,
			},
		}
	}

	// run the callback to modify the account
	f(object)

	if object != nil {
		txn.txn.Insert(addr.Bytes(), object)
	}
}

func (txn *Txn) AddSealingReward(addr common.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		if object.suicide {
			*object = *newStateObject()
			object.account.Balance.SetBytes(balance.Bytes())
		} else {
			object.account.Balance.Add(object.account.Balance, balance)
		}
	})
}

// AddBalance adds balance
func (txn *Txn) AddBalance(addr common.Address, balance *big.Int) {
	//fmt.Printf("ADD BALANCE: %s %d\n", addr.String(), balance.Uint64())

	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance.Add(object.account.Balance, balance)
	})
}

// SubBalance reduces the balance
func (txn *Txn) SubBalance(addr common.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance.Sub(object.account.Balance, balance)
	})
}

// SetBalance sets the balance
func (txn *Txn) SetBalance(addr common.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Balance.SetBytes(balance.Bytes())
	})
}

// GetBalance returns the balance of an address
func (txn *Txn) GetBalance(addr common.Address) *big.Int {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return big.NewInt(0)
	}
	return object.account.Balance
}

// AddLog adds a new log
func (txn *Txn) AddLog(log *types.Log) {
	var logs []*types.Log

	data, exists := txn.txn.Get(logIndex)
	if !exists {
		logs = []*types.Log{}
	} else {
		logs = data.([]*types.Log)
	}

	logs = append(logs, log)
	txn.txn.Insert(logIndex, logs)
}

// State

func isZeros(b []byte) bool {
	for _, i := range b {
		if i != 0x0 {
			return false
		}
	}
	return true
}

// SetState change the state of an address
func (txn *Txn) SetState(addr common.Address, key, value common.Hash) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		if object.txn == nil {
			object.txn = iradix.New().Txn()
		}

		if isZeros(value.Bytes()) {
			object.txn.Insert(hashit(key.Bytes()), nil)
		} else {
			object.txn.Insert(hashit(key.Bytes()), value.Bytes())
		}
	})
}

// GetState returns the state of the address at a given hash
func (txn *Txn) GetState(addr common.Address, hash common.Hash) common.Hash {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return common.Hash{}
	}

	k := hashit(hash.Bytes())

	if object.txn != nil {
		if val, ok := object.txn.Get(k); ok {
			if val == nil {
				return common.Hash{}
			}
			return common.BytesToHash(val.([]byte))
		}
	}
	return object.GetCommitedState(common.BytesToHash(k))
}

// Nonce

// SetNonce reduces the balance
func (txn *Txn) SetNonce(addr common.Address, nonce uint64) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.account.Nonce = nonce
	})
}

// GetNonce returns the nonce of an addr
func (txn *Txn) GetNonce(addr common.Address) uint64 {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return 0
	}
	return object.account.Nonce
}

// Code

// SetCode sets the code for an address
func (txn *Txn) SetCode(addr common.Address, code []byte) {
	txn.upsertAccount(addr, true, func(object *stateObject) {
		object.account.CodeHash = crypto.Keccak256Hash(code).Bytes()
		object.dirtyCode = true
		object.code = code
	})
}

func (txn *Txn) GetCode(addr common.Address) []byte {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil
	}

	if object.dirtyCode {
		return object.code
	}
	code, _ := txn.state.GetCode(common.BytesToHash(object.account.CodeHash))
	return code
}

func (txn *Txn) GetCodeSize(addr common.Address) int {
	return len(txn.GetCode(addr))
}

func (txn *Txn) GetCodeHash(addr common.Address) common.Hash {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return common.Hash{}
	}
	return common.BytesToHash(object.account.CodeHash)
}

// Suicide

// Suicide marks the given account as suicided
func (txn *Txn) Suicide(addr common.Address) bool {
	var suicided bool
	txn.upsertAccount(addr, false, func(object *stateObject) {
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
func (txn *Txn) HasSuicided(addr common.Address) bool {
	object, exists := txn.getStateObject(addr)
	return exists && object.suicide
}

// Refund
func (txn *Txn) AddRefund(gas uint64) {
	refund := txn.GetRefund() + gas
	txn.txn.Insert(refundIndex, refund)
}

func (txn *Txn) SubRefund(gas uint64) {
	refund := txn.GetRefund() - gas
	txn.txn.Insert(refundIndex, refund)
}

func (txn *Txn) Logs() []*types.Log {
	data, exists := txn.txn.Get(logIndex)
	if !exists {
		return nil
	}
	return data.([]*types.Log)
}

func (txn *Txn) GetRefund() uint64 {
	data, exists := txn.txn.Get(refundIndex)
	if !exists {
		return 0
	}
	return data.(uint64)
}

func (txn *Txn) IntermediateRoot(bool) common.Hash {
	return common.Hash{}
}

// GetCommittedState returns the state of the address in the trie
func (txn *Txn) GetCommittedState(addr common.Address, hash common.Hash) common.Hash {
	obj, ok := txn.getStateObject(addr)
	if !ok {
		return common.Hash{}
	}
	return obj.GetCommitedState(common.BytesToHash(hashit(hash.Bytes())))
}

// TODO, check panics with this ones

func (txn *Txn) Exist(addr common.Address) bool {
	_, exists := txn.getStateObject(addr)
	return exists
}

func (txn *Txn) Empty(addr common.Address) bool {
	obj, exists := txn.getStateObject(addr)
	if !exists {
		return true
	}
	return obj.Empty()
}

func newStateObject() *stateObject {
	return &stateObject{
		account: &Account{
			Balance:  big.NewInt(0),
			trie:     trie.NewTrie(),
			CodeHash: emptyCodeHash,
			Root:     emptyStateHash,
		},
	}
}

func (txn *Txn) CreateAccount(addr common.Address) {
	obj := &stateObject{
		account: &Account{
			Balance:  big.NewInt(0),
			trie:     trie.NewTrie(),
			CodeHash: emptyCodeHash,
			Root:     emptyStateHash,
		},
	}

	prev, ok := txn.getStateObject(addr)
	if ok {
		obj.account.Balance.SetBytes(prev.account.Balance.Bytes())
	}

	txn.txn.Insert(addr.Bytes(), obj)
}

func hashit(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)
	return h.Sum(nil)
}

// IntermediateCommit runs after each tx is completed to set to false those accounts that
// have been deleted
func (txn *Txn) IntermediateCommit(deleteEmptyObjects bool) {
	// between different transactions and before the final commit we must
	// check all the txns and mark as deleted the ones removed

	remove := [][]byte{}
	txn.txn.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*stateObject)
		if !ok {
			return false
		}
		if a.suicide || a.Empty() && deleteEmptyObjects {
			remove = append(remove, k)
		}
		return false
	})

	for _, k := range remove {
		v, ok := txn.txn.Get(k)
		if !ok {
			panic("it should not happen")
		}
		obj, ok := v.(*stateObject)
		if !ok {
			panic("it should not happen")
		}

		obj2 := obj.Copy()
		obj2.deleted = true
		txn.txn.Insert(k, obj2)
	}

	// delete refunds
	txn.txn.Delete(refundIndex)
}

func (txn *Txn) Commit(deleteEmptyObjects bool) (*State, []byte) {
	txn.IntermediateCommit(deleteEmptyObjects)

	x := txn.txn.Commit()

	tt := txn.state.getRoot().Txn()

	/*
		fmt.Println("##################################################################################")

		x.Root().Walk(func(k []byte, v interface{}) bool {
			a, ok := v.(*stateObject)
			if !ok {
				// We also have logs, avoid those
				return false
			}
			fmt.Printf("# ----------------- %s -------------------\n", hexutil.Encode(k))
			fmt.Printf("# Deleted: %v, Suicided: %v\n", a.deleted, a.suicide)
			fmt.Printf("# Balance: %d\n", a.account.Balance.Uint64())
			fmt.Printf("# Nonce: %s\n", strconv.Itoa(int(a.account.Nonce)))
			fmt.Printf("# Code hash: %s\n", hexutil.Encode(a.account.CodeHash))
			fmt.Printf("# State root: %s\n", a.account.Root.String())
			if a.txn != nil {
				a.txn.Root().Walk(func(k []byte, v interface{}) bool {
					if v == nil {
						fmt.Printf("#\t%s: EMPTY\n", hexutil.Encode(k))
					} else {
						fmt.Printf("#\t%s: %s\n", hexutil.Encode(k), hexutil.Encode(v.([]byte)))
					}
					return false
				})
			}
			return false
		})
		fmt.Println("##################################################################################")
	*/

	batch := txn.state.storage.Batch()

	x.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*stateObject)
		if !ok {
			// We also have logs, avoid those
			return false
		}

		if a.deleted {
			tt.Delete(hashit(k))
			return false
		}

		// compute first the state changes
		if a.txn != nil {
			localTxn := a.account.trie.Txn()

			// Apply all the changes
			a.txn.Root().Walk(func(k []byte, v interface{}) bool {
				if v == nil {
					localTxn.Delete(k)
				} else {
					vv, _ := rlp.EncodeToBytes(bytes.TrimLeft(v.([]byte), "\x00"))
					localTxn.Insert(k, vv)
				}
				return false
			})

			accountStateRoot := localTxn.Hash(batch)
			subTrie := localTxn.Commit()

			a.account.Root = common.BytesToHash(accountStateRoot)
			a.account.trie = subTrie
		}

		if a.dirtyCode {
			txn.state.SetCode(common.BytesToHash(a.account.CodeHash), a.code)
		}

		data, err := rlp.EncodeToBytes(a.account)
		if err != nil {
			panic(err)
		}

		tt.Insert(hashit(k), data)
		return false
	})

	t := tt.Commit()

	hash := tt.Hash(batch)

	batch.Write()

	newState := &State{
		storage: txn.state.storage,
		root:    unsafe.Pointer(t),
		code:    map[string][]byte{},
	}

	// copy all the code
	// TODO, Move to trie
	for k, v := range txn.state.code {
		newState.code[k] = v
	}

	// atomic.StorePointer(&txn.state.root, unsafe.Pointer(t))
	return newState, hash
}
