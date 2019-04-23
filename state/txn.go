package state

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/rlp"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	iradix "github.com/hashicorp/go-immutable-radix"
)

var (
	ErrInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

// var emptyCodeHash = crypto.Keccak256(nil)

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

// Txn is a reference of the state
type Txn struct {
	snapshot   Snapshot
	state      State
	snapshots  []*iradix.Tree
	txn        *iradix.Txn
	gas        uint64
	initialGas uint64
}

func NewTxn(state State, snapshot Snapshot) *Txn {
	return newTxn(state, snapshot)
}

func newTxn(state State, snapshot Snapshot) *Txn {
	i := iradix.New()

	return &Txn{
		snapshot:  snapshot,
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
	return object.Account, true
}

func (txn *Txn) getStateObject(addr common.Address) (*StateObject, bool) {
	val, exists := txn.txn.Get(addr.Bytes())
	if exists {
		obj := val.(*StateObject)
		if obj.Deleted {
			return nil, false
		}
		return obj.Copy(), true
	}

	data, ok := txn.snapshot.Get(hashit(addr.Bytes()))
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
		account.Trie = txn.state.NewSnapshot()
	} else {
		account.Trie, err = txn.state.NewSnapshotAt(account.Root)
		if err != nil {
			return nil, false
		}
	}

	obj := &StateObject{
		Account: account.Copy(),
	}
	return obj, true
}

func (txn *Txn) upsertAccount(addr common.Address, create bool, f func(object *StateObject)) {
	object, exists := txn.getStateObject(addr)
	if !exists && create {
		object = &StateObject{
			Account: &Account{
				Balance:  big.NewInt(0),
				Trie:     txn.state.NewSnapshot(),
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
	txn.upsertAccount(addr, true, func(object *StateObject) {
		if object.Suicide {
			*object = *newStateObject(txn)
			object.Account.Balance.SetBytes(balance.Bytes())
		} else {
			object.Account.Balance.Add(object.Account.Balance, balance)
		}
	})
}

// AddBalance adds balance
func (txn *Txn) AddBalance(addr common.Address, balance *big.Int) {
	/*
		if balance.Sign() == 0 {
			return
		}
	*/
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Balance.Add(object.Account.Balance, balance)
	})
}

// SubBalance reduces the balance
func (txn *Txn) SubBalance(addr common.Address, balance *big.Int) {
	if balance.Sign() == 0 {
		return
	}
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Balance.Sub(object.Account.Balance, balance)
	})
}

// SetBalance sets the balance
func (txn *Txn) SetBalance(addr common.Address, balance *big.Int) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Balance.SetBytes(balance.Bytes())
	})
}

// GetBalance returns the balance of an address
func (txn *Txn) GetBalance(addr common.Address) *big.Int {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return big.NewInt(0)
	}
	return object.Account.Balance
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
	txn.upsertAccount(addr, true, func(object *StateObject) {
		if object.Txn == nil {
			object.Txn = iradix.New().Txn()
		}

		if isZeros(value.Bytes()) {
			object.Txn.Insert(hashit(key.Bytes()), nil)
		} else {
			object.Txn.Insert(hashit(key.Bytes()), value.Bytes())
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

	if object.Txn != nil {
		if val, ok := object.Txn.Get(k); ok {
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
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Nonce = nonce
	})
}

// GetNonce returns the nonce of an addr
func (txn *Txn) GetNonce(addr common.Address) uint64 {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return 0
	}
	return object.Account.Nonce
}

// Code

// SetCode sets the code for an address
func (txn *Txn) SetCode(addr common.Address, code []byte) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.CodeHash = crypto.Keccak256Hash(code).Bytes()
		object.DirtyCode = true
		object.Code = code
	})
}

func (txn *Txn) GetCode(addr common.Address) []byte {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil
	}

	if object.DirtyCode {
		return object.Code
	}
	code, _ := txn.state.GetCode(common.BytesToHash(object.Account.CodeHash))
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
	return common.BytesToHash(object.Account.CodeHash)
}

// Suicide

// Suicide marks the given account as suicided
func (txn *Txn) Suicide(addr common.Address) bool {
	var suicided bool
	txn.upsertAccount(addr, false, func(object *StateObject) {
		if object == nil || object.Suicide {
			suicided = false
		} else {
			suicided = true
			object.Suicide = true
			object.Account.Balance = new(big.Int)
		}
	})
	return suicided
}

// HasSuicided returns true if the account suicided
func (txn *Txn) HasSuicided(addr common.Address) bool {
	object, exists := txn.getStateObject(addr)
	return exists && object.Suicide
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

func newStateObject(txn *Txn) *StateObject {
	return &StateObject{
		Account: &Account{
			Balance:  big.NewInt(0),
			Trie:     txn.state.NewSnapshot(),
			CodeHash: emptyCodeHash,
			Root:     emptyStateHash,
		},
	}
}

func (txn *Txn) CreateAccount(addr common.Address) {
	obj := &StateObject{
		Account: &Account{
			Balance:  big.NewInt(0),
			Trie:     txn.state.NewSnapshot(),
			CodeHash: emptyCodeHash,
			Root:     emptyStateHash,
		},
	}

	prev, ok := txn.getStateObject(addr)
	if ok {
		obj.Account.Balance.SetBytes(prev.Account.Balance.Bytes())
	}

	txn.txn.Insert(addr.Bytes(), obj)
}

func hashit(k []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(k)
	return h.Sum(nil)
}

func (txn *Txn) cleanDeleteObjects(deleteEmptyObjects bool) {
	remove := [][]byte{}
	txn.txn.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*StateObject)
		if !ok {
			return false
		}
		if a.Suicide || a.Empty() && deleteEmptyObjects {
			remove = append(remove, k)
		}
		return false
	})

	for _, k := range remove {
		v, ok := txn.txn.Get(k)
		if !ok {
			panic("it should not happen")
		}
		obj, ok := v.(*StateObject)
		if !ok {
			panic("it should not happen")
		}

		obj2 := obj.Copy()
		obj2.Deleted = true
		txn.txn.Insert(k, obj2)
	}

	// delete refunds
	txn.txn.Delete(refundIndex)
}

func (txn *Txn) Commit(deleteEmptyObjects bool) (Snapshot, []byte) {
	txn.cleanDeleteObjects(deleteEmptyObjects)

	x := txn.txn.Commit()

	/*
		fmt.Println("##################################################################################")

		x.Root().Walk(func(k []byte, v interface{}) bool {
			a, ok := v.(*StateObject)
			if !ok {
				// We also have logs, avoid those
				return false
			}
			fmt.Printf("# ----------------- %s -------------------\n", hexutil.Encode(k))
			fmt.Printf("# Deleted: %v, Suicided: %v\n", a.Deleted, a.Suicide)
			fmt.Printf("# Balance: %s\n", a.Account.Balance.String())
			fmt.Printf("# Nonce: %s\n", strconv.Itoa(int(a.Account.Nonce)))
			fmt.Printf("# Code hash: %s\n", hexutil.Encode(a.Account.CodeHash))
			fmt.Printf("# State root: %s\n", a.Account.Root.String())
			if a.Txn != nil {
				a.Txn.Root().Walk(func(k []byte, v interface{}) bool {
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

	t, hash := txn.snapshot.Commit(x)
	return t, hash
}
