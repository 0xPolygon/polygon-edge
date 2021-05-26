package state

import (
	"fmt"
	"hash"
	"math/big"
	"strconv"

	iradix "github.com/hashicorp/go-immutable-radix"
	lru "github.com/hashicorp/golang-lru"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/types"
)

var emptyStateHash = types.StringToHash("0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421")

var (
	// logIndex is the index of the logs in the trie
	logIndex = types.BytesToHash([]byte{2}).Bytes()

	// refundIndex is the index of the refund
	refundIndex = types.BytesToHash([]byte{3}).Bytes()
)

// Txn is a reference of the state
type Txn struct {
	snapshot  Snapshot
	state     State
	snapshots []*iradix.Tree
	txn       *iradix.Txn
	codeCache *lru.Cache
	hash      *keccak.Keccak
}

func NewTxn(state State, snapshot Snapshot) *Txn {
	return newTxn(state, snapshot)
}

func newTxn(state State, snapshot Snapshot) *Txn {
	i := iradix.New()

	codeCache, _ := lru.New(20)

	return &Txn{
		snapshot:  snapshot,
		state:     state,
		snapshots: []*iradix.Tree{},
		txn:       i.Txn(),
		codeCache: codeCache,
		hash:      keccak.NewKeccak256(),
	}
}

func (txn *Txn) hashit(src []byte) []byte {
	txn.hash.Reset()
	txn.hash.Write(src)
	// hashit is used to make queries so we do not need to
	// make copies of the result
	return txn.hash.Read()
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
func (txn *Txn) GetAccount(addr types.Address) (*Account, bool) {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil, false
	}
	return object.Account, true
}

func (txn *Txn) getStateObject(addr types.Address) (*StateObject, bool) {
	val, exists := txn.txn.Get(addr.Bytes())
	if exists {
		obj := val.(*StateObject)
		if obj.Deleted {
			return nil, false
		}
		return obj.Copy(), true
	}

	data, ok := txn.snapshot.Get(txn.hashit(addr.Bytes()))
	if !ok {
		return nil, false
	}

	var err error
	var account Account
	if err = account.UnmarshalRlp(data); err != nil {
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

func (txn *Txn) upsertAccount(addr types.Address, create bool, f func(object *StateObject)) {
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

func (txn *Txn) AddSealingReward(addr types.Address, balance *big.Int) {
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
func (txn *Txn) AddBalance(addr types.Address, balance *big.Int) {
	//fmt.Printf("ADD BALANCE: %s %s\n", addr.String(), balance.String())
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
func (txn *Txn) SubBalance(addr types.Address, balance *big.Int) {
	//fmt.Printf("SUB BALANCE: %s %s\n", addr.String(), balance.String())

	if balance.Sign() == 0 {
		return
	}
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Balance.Sub(object.Account.Balance, balance)
	})
}

// SetBalance sets the balance
func (txn *Txn) SetBalance(addr types.Address, balance *big.Int) {
	//fmt.Printf("SET BALANCE: %s %s\n", addr.String(), balance.String())
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Balance.SetBytes(balance.Bytes())
	})
}

// GetBalance returns the balance of an address
func (txn *Txn) GetBalance(addr types.Address) *big.Int {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return big.NewInt(0)
	}
	return object.Account.Balance
}

func (txn *Txn) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	log := &types.Log{
		Address: addr,
		Topics:  topics,
	}
	log.Data = append(log.Data, data...)

	var logs []*types.Log
	val, exists := txn.txn.Get(logIndex)
	if !exists {
		logs = []*types.Log{}
	} else {
		logs = val.([]*types.Log)
	}

	logs = append(logs, log)
	txn.txn.Insert(logIndex, logs)
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

var zeroHash types.Hash

func (txn *Txn) SetStorage(addr types.Address, key types.Hash, value types.Hash, config *chain.ForksInTime) (status runtime.StorageStatus) {
	oldValue := txn.GetState(addr, key)
	if oldValue == value {
		return runtime.StorageUnchanged
	}

	current := oldValue
	original := txn.GetCommittedState(addr, key)

	txn.SetState(addr, key, value)

	legacyGasMetering := !config.Istanbul && (config.Petersburg || !config.Constantinople)

	if legacyGasMetering {
		status = runtime.StorageModified
		if oldValue == zeroHash {
			return runtime.StorageAdded
		} else if value == zeroHash {
			txn.AddRefund(15000)
			return runtime.StorageDeleted
		}
		return runtime.StorageModified
	}

	if original == current {
		if original == zeroHash { // create slot (2.1.1)
			return runtime.StorageAdded
		}
		if value == zeroHash { // delete slot (2.1.2b)
			txn.AddRefund(15000)
			return runtime.StorageDeleted
		}
		return runtime.StorageModified
	}
	if original != zeroHash {
		if current == zeroHash { // recreate slot (2.2.1.1)
			txn.SubRefund(15000)
		} else if value == zeroHash { // delete slot (2.2.1.2)
			txn.AddRefund(15000)
		}
	}
	if original == value {
		if original == zeroHash { // reset to original inexistent slot (2.2.2.1)
			if config.Istanbul {
				txn.AddRefund(19200)
			} else {
				txn.AddRefund(19800)
			}
		} else { // reset to original existing slot (2.2.2.2)
			if config.Istanbul {
				txn.AddRefund(4200)
			} else {
				txn.AddRefund(4800)
			}
		}
	}
	return runtime.StorageModifiedAgain
}

// SetState change the state of an address
func (txn *Txn) SetState(addr types.Address, key, value types.Hash) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		if object.Txn == nil {
			object.Txn = iradix.New().Txn()
		}

		if value == zeroHash {
			object.Txn.Insert(key.Bytes(), nil)
		} else {
			object.Txn.Insert(key.Bytes(), value.Bytes())
		}
	})
}

// GetState returns the state of the address at a given hash
func (txn *Txn) GetState(addr types.Address, hash types.Hash) types.Hash {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return types.Hash{}
	}

	if object.Txn != nil {
		if val, ok := object.Txn.Get(hash.Bytes()); ok {
			if val == nil {
				return types.Hash{}
			}
			return types.BytesToHash(val.([]byte))
		}
	}

	k := txn.hashit(hash.Bytes())
	return object.GetCommitedState(types.BytesToHash(k))
}

// Nonce

// IncrNonce increases the nonce of the address
func (txn *Txn) IncrNonce(addr types.Address) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Nonce++
	})
}

// SetNonce reduces the balance
func (txn *Txn) SetNonce(addr types.Address, nonce uint64) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.Nonce = nonce
	})
}

// GetNonce returns the nonce of an addr
func (txn *Txn) GetNonce(addr types.Address) uint64 {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return 0
	}
	return object.Account.Nonce
}

// Code

// SetCode sets the code for an address
func (txn *Txn) SetCode(addr types.Address, code []byte) {
	txn.upsertAccount(addr, true, func(object *StateObject) {
		object.Account.CodeHash = crypto.Keccak256(code)
		object.DirtyCode = true
		object.Code = code
	})
}

func (txn *Txn) GetCode(addr types.Address) []byte {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return nil
	}
	if object.DirtyCode {
		return object.Code
	}
	// TODO; Should we move this to state?
	v, ok := txn.codeCache.Get(addr)
	if ok {
		return v.([]byte)
	}
	code, _ := txn.state.GetCode(types.BytesToHash(object.Account.CodeHash))
	txn.codeCache.Add(addr, code)
	return code
}

func (txn *Txn) GetCodeSize(addr types.Address) int {
	return len(txn.GetCode(addr))
}

func (txn *Txn) GetCodeHash(addr types.Address) types.Hash {
	object, exists := txn.getStateObject(addr)
	if !exists {
		return types.Hash{}
	}
	return types.BytesToHash(object.Account.CodeHash)
}

// Suicide marks the given account as suicided
func (txn *Txn) Suicide(addr types.Address) bool {
	var suicided bool
	txn.upsertAccount(addr, false, func(object *StateObject) {
		if object == nil || object.Suicide {
			suicided = false
		} else {
			suicided = true
			object.Suicide = true
		}
		if object != nil {
			object.Account.Balance = new(big.Int)
		}
	})
	return suicided
}

// HasSuicided returns true if the account suicided
func (txn *Txn) HasSuicided(addr types.Address) bool {
	object, exists := txn.getStateObject(addr)
	return exists && object.Suicide
}

// Refund
func (txn *Txn) AddRefund(gas uint64) {
	// fmt.Printf("=-----------ADD REFUND: %d\n", gas)

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
	txn.txn.Delete(logIndex)
	return data.([]*types.Log)
}

func (txn *Txn) GetRefund() uint64 {
	data, exists := txn.txn.Get(refundIndex)
	if !exists {
		return 0
	}
	return data.(uint64)
}

// GetCommittedState returns the state of the address in the trie
func (txn *Txn) GetCommittedState(addr types.Address, hash types.Hash) types.Hash {
	obj, ok := txn.getStateObject(addr)
	if !ok {
		return types.Hash{}
	}
	return obj.GetCommitedState(types.BytesToHash(txn.hashit(hash.Bytes())))
}

func (txn *Txn) TouchAccount(addr types.Address) {
	txn.upsertAccount(addr, true, func(obj *StateObject) {

	})
}

// TODO, check panics with this ones

func (txn *Txn) Exist(addr types.Address) bool {
	_, exists := txn.getStateObject(addr)
	return exists
}

func (txn *Txn) Empty(addr types.Address) bool {
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

func (txn *Txn) CreateAccount(addr types.Address) {
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

func (txn *Txn) CleanDeleteObjects(deleteEmptyObjects bool) {
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

func (txn *Txn) show(i *iradix.Txn) {
	fmt.Println("##################################################################################")

	i.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*StateObject)
		if !ok {
			// We also have logs, avoid those
			return false
		}
		fmt.Printf("# ----------------- %s -------------------\n", hex.EncodeToHex(k))
		fmt.Printf("# Deleted: %v, Suicided: %v\n", a.Deleted, a.Suicide)
		fmt.Printf("# Balance: %s\n", a.Account.Balance.String())
		fmt.Printf("# Nonce: %s\n", strconv.Itoa(int(a.Account.Nonce)))
		fmt.Printf("# Code hash: %s\n", hex.EncodeToHex(a.Account.CodeHash))
		fmt.Printf("# State root: %s\n", a.Account.Root.String())
		if a.Txn != nil {
			a.Txn.Root().Walk(func(k []byte, v interface{}) bool {
				if v == nil {
					fmt.Printf("#\t%s: EMPTY\n", hex.EncodeToHex(k))
				} else {
					fmt.Printf("#\t%s: %s\n", hex.EncodeToHex(k), hex.EncodeToHex(v.([]byte)))
				}
				return false
			})
		}
		return false
	})
	fmt.Println("##################################################################################")
}

func show(objs []*Object) {
	fmt.Println("##################################################################################")

	for _, obj := range objs {
		fmt.Printf("# ----------------- %s -------------------\n", obj.Address.String())
		fmt.Printf("# Deleted: %v\n", obj.Deleted)
		fmt.Printf("# Balance: %s\n", obj.Balance.String())
		fmt.Printf("# Nonce: %s\n", strconv.Itoa(int(obj.Nonce)))
		fmt.Printf("# Code hash: %s\n", obj.CodeHash.String())
		fmt.Printf("# State root: %s\n", obj.Root.String())

		for _, entry := range obj.Storage {
			if entry.Deleted {
				fmt.Printf("#\t%s: EMPTY\n", hex.EncodeToHex(entry.Key))
			} else {
				fmt.Printf("#\t%s: %s\n", hex.EncodeToHex(entry.Key), hex.EncodeToHex(entry.Val))
			}
		}
	}
	fmt.Println("##################################################################################")
}

func (txn *Txn) Commit(deleteEmptyObjects bool) (Snapshot, []byte) {
	txn.CleanDeleteObjects(deleteEmptyObjects)

	x := txn.txn.Commit()

	// Do a more complex thing for now
	objs := []*Object{}
	x.Root().Walk(func(k []byte, v interface{}) bool {
		a, ok := v.(*StateObject)
		if !ok {
			// We also have logs, avoid those
			return false
		}

		obj := &Object{
			Nonce:     a.Account.Nonce,
			Address:   types.BytesToAddress(k),
			Balance:   a.Account.Balance,
			Root:      a.Account.Root,
			CodeHash:  types.BytesToHash(a.Account.CodeHash),
			DirtyCode: a.DirtyCode,
			Code:      a.Code,
		}
		if a.Deleted {
			obj.Deleted = true
		} else {
			if a.Txn != nil {
				a.Txn.Root().Walk(func(k []byte, v interface{}) bool {
					store := &StorageObject{Key: k}
					if v == nil {
						store.Deleted = true
					} else {
						store.Val = v.([]byte)
					}
					obj.Storage = append(obj.Storage, store)
					return false
				})
			}
		}

		objs = append(objs, obj)
		return false
	})
	// show(objs)

	t, hash := txn.snapshot.Commit(objs)
	return t, hash
}

type hashImpl interface {
	hash.Hash
	Read(b []byte) (int, error)
}

func extendByteSlice(b []byte, needLen int) []byte {
	b = b[:cap(b)]
	if n := needLen - cap(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}
	return b[:needLen]
}
