package state

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"unsafe"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	iradix "github.com/hashicorp/go-immutable-radix"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/evm"
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

func (txn *Txn) Apply(msg *types.Message, env *evm.Env, gasTable chain.GasTable, config chain.ForksInTime, getHash evm.GetHashByNumber, gasPool GasPool, dryRun bool, builtins map[common.Address]*evm.Precompiled2) (uint64, bool, error) {
	s := txn.Snapshot()
	gas, failed, err := txn.apply(msg, env, gasTable, config, getHash, gasPool, dryRun, builtins)
	if err != nil {
		txn.RevertToSnapshot(s)
	}

	//fmt.Println("-- failed --")
	//fmt.Println(failed)

	return gas, failed, err
}

func (txn *Txn) apply(msg *types.Message, env *evm.Env, gasTable chain.GasTable, config chain.ForksInTime, getHash evm.GetHashByNumber, gasPool GasPool, dryRun bool, builtins map[common.Address]*evm.Precompiled2) (uint64, bool, error) {
	// transition
	s := txn.Snapshot()

	// check nonce is correct (pre-check)
	if msg.CheckNonce() {
		nonce := txn.GetNonce(msg.From())
		if nonce < msg.Nonce() {
			return 0, false, fmt.Errorf("too high %d < %d", nonce, msg.Nonce())
		} else if nonce > msg.Nonce() {
			return 0, false, fmt.Errorf("too low %d > %d", nonce, msg.Nonce())
		}
	}

	// buy gas
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(msg.Gas()), msg.GasPrice())
	if txn.GetBalance(msg.From()).Cmp(mgval) < 0 {
		return 0, false, ErrInsufficientBalanceForGas
	}

	// check if there is space for this tx in the gaspool
	if err := gasPool.SubGas(msg.Gas()); err != nil {
		return 0, false, err
	}

	// restart the txn gas
	txn.gas = msg.Gas()

	txn.initialGas = msg.Gas()
	txn.SubBalance(msg.From(), mgval)

	contractCreation := msg.To() == nil

	// compute intrinsic gas for the tx (data, contract creation, call...)
	var txGas uint64
	if contractCreation && config.Homestead { // TODO, homestead thing
		txGas = chain.TxGasContractCreation
	} else {
		txGas = chain.TxGas
	}

	data := msg.Data()
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		if (math.MaxUint64-txGas)/chain.TxDataNonZeroGas < nz {
			return 0, false, vm.ErrOutOfGas
		}
		txGas += nz * chain.TxDataNonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-txGas)/chain.TxDataZeroGas < z {
			return 0, false, vm.ErrOutOfGas
		}
		txGas += z * chain.TxDataZeroGas
	}

	// reduce the intrinsic gas from the total gas
	if txn.gas < txGas {
		return 0, false, vm.ErrOutOfGas
	}

	txn.gas -= txGas

	sender := msg.From()

	var vmerr error

	if !dryRun {
		e := evm.NewEVM(txn, env, config, gasTable, getHash)
		e.SetPrecompiled(builtins)

		if contractCreation {
			//fmt.Println("- one ")
			_, txn.gas, vmerr = e.Create(sender, msg.Data(), msg.Value(), txn.gas)
		} else {
			//fmt.Println("- two")
			txn.SetNonce(msg.From(), txn.GetNonce(msg.From())+1)
			_, txn.gas, vmerr = e.Call(sender, *msg.To(), msg.Data(), msg.Value(), txn.gas)
		}

		//fmt.Println("-- vm err --")
		//fmt.Println(vmerr)

		if vmerr != nil {
			if vmerr == evm.ErrNotEnoughFunds {
				txn.RevertToSnapshot(s)
				return 0, false, vmerr
			}
		}
	}

	// refund
	// Apply refund counter, capped to half of the used gas.
	refund := txn.gasUsed() / 2

	if refund > txn.GetRefund() {
		refund = txn.GetRefund()
	}

	txn.gas += refund

	//fmt.Println("-- get refund --")
	//fmt.Println(txn.GetRefund())

	//fmt.Println("-- refund --")
	//fmt.Println(refund)

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(txn.gas), msg.GasPrice())

	//fmt.Println("-- remaining --")
	// fmt.Println(remaining)

	txn.AddBalance(msg.From(), remaining)

	// pay the coinbase
	txn.AddBalance(env.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(txn.gasUsed()), msg.GasPrice()))

	// Return remaining gas to the pool for the block
	gasPool.AddGas(txn.gas)

	//fmt.Println("-- ## VMerr --")
	//fmt.Println(vmerr)

	return txn.gasUsed(), vmerr != nil, nil
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
	txn       *trie.Txn
}

func (s *stateObject) Empty() bool {
	return s.account.Nonce == 0 && s.account.Balance.Sign() == 0 && bytes.Equal(s.account.CodeHash, emptyCodeHash)
}

// Copy makes a copy of the state object
func (s *stateObject) Copy() *stateObject {
	ss := new(stateObject)

	// copy account
	ss.account = s.account.Copy()

	if s.txn != nil {
		ss.txn = s.txn.Copy()
	}

	ss.suicide = s.suicide
	ss.deleted = s.deleted
	ss.dirtyCode = s.dirtyCode
	ss.code = s.code

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

	account, ok := data.(*Account)
	if !ok {
		panic("XX")
	}

	obj := &stateObject{
		account: account.Copy(),
		// storage: map[common.Hash]common.Hash{},
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
			object.txn = object.account.trie.Txn()
		}

		if isZeros(value.Bytes()) {
			object.txn.Delete(hashit(key.Bytes()))
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

	var f func(k []byte) (interface{}, bool)

	if object.txn != nil {
		f = object.txn.Get
	} else {
		f = object.account.trie.Get
	}

	val, ok := f(hashit(hash.Bytes()))
	if ok {
		return common.BytesToHash(val.([]byte))
	}
	return common.Hash{}
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
	val, ok := obj.account.trie.Get(hashit(hash.Bytes()))
	if !ok {
		return common.Hash{}
	}
	return common.BytesToHash(val.([]byte))
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

		// fmt.Println(hexutil.Encode(k))

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

	// fmt.Printf("EIP enabled: %v\n", deleteEmptyObjects)

	tt := txn.state.getRoot().Txn()

	h1 := sha3.NewLegacyKeccak256()

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

			return false
		})
		fmt.Println("##################################################################################")
	*/

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
			subTrie := a.txn.Commit()
			accountStateRoot := subTrie.Root().Hash()

			a.account.Root = common.BytesToHash(accountStateRoot)
			a.account.trie = subTrie
		}

		if a.dirtyCode {
			txn.state.SetCode(common.BytesToHash(a.account.CodeHash), a.code)
		}

		h1.Reset()
		h1.Write(k)
		kk := h1.Sum(nil)

		tt.Insert(kk, a.account)

		return false
	})

	t := tt.Commit()
	hash := tt.Hash()

	newState := &State{
		root: unsafe.Pointer(t),
		code: map[string][]byte{},
	}

	// copy all the code
	for k, v := range txn.state.code {
		newState.code[k] = v
	}

	// atomic.StorePointer(&txn.state.root, unsafe.Pointer(t))
	return newState, hash
}
