package state

import (
	"fmt"
	"math/big"

	"github.com/umbracle/minimal/types"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/crypto"
	"github.com/umbracle/minimal/state/runtime"
)

const (
	spuriousDragonMaxCodeSize = 24576
)

var (
	errorVMOutOfGas = fmt.Errorf("out of gas")
)

var emptyCodeHashTwo = types.BytesToHash(crypto.Keccak256(nil))

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) types.Hash

type Executor struct {
	runtimes []runtime.Runtime
	callback func(t *Transition)
}

func NewExecutor() *Executor {
	return &Executor{
		runtimes: []runtime.Runtime{},
	}
}

func (e *Executor) SetRuntime(r runtime.Runtime) {
	e.runtimes = append(e.runtimes, r)
}

func (e *Executor) SetCallback(callback func(t *Transition)) {
	e.callback = callback
}

func (e *Executor) NewTransition(txn *Txn, getHash GetHashByNumber, ctx runtime.TxContext, config chain.ForksInTime) *Transition {
	return &Transition{
		r:       e,
		ctx:     ctx,
		state:   txn,
		getHash: getHash,
		config:  &config,
		gasPool: uint64(ctx.GasLimit),
	}
}

type Transition struct {
	r       *Executor
	config  *chain.ForksInTime
	state   *Txn
	getHash GetHashByNumber
	ctx     runtime.TxContext
	gasPool uint64
	txnHash types.Hash
}

func (t *Transition) SetTxnHash(txn types.Hash) {
	t.txnHash = txn
}

func (t *Transition) GetTxnHash() types.Hash {
	return t.txnHash
}

func (t *Transition) subGasPool(amount uint64) error {
	if t.gasPool < amount {
		return fmt.Errorf("gas limit reached in the pool")
	}
	t.gasPool -= amount
	return nil
}

func (t *Transition) addGasPool(amount uint64) {
	t.gasPool += amount
}

func (t *Transition) SetTxn(txn *Txn) {
	t.state = txn
}

func (t *Transition) Txn() *Txn {
	return t.state
}

// Apply applies a new transaction
func (t *Transition) Apply(msg *types.Transaction) (uint64, bool, error) {
	s := t.state.Snapshot()
	gas, failed, err := t.apply(msg)
	if err != nil {
		t.state.RevertToSnapshot(s)
	}

	if err == nil {
		// apply other possible changes to the state after the transaction
		if t.r.callback != nil {
			t.r.callback(t)
		}
	}

	// e.addGasPool(gas)
	return gas, failed, err
}

func (t *Transition) Context() runtime.TxContext {
	return t.ctx
}

func (t *Transition) transactionGasCost(msg *types.Transaction) uint64 {
	cost := uint64(0)

	// Contract creation is only paid on the homestead fork
	if msg.IsContractCreation() && t.config.Homestead {
		cost += 53000
	} else {
		cost += 21000
	}

	payload := msg.Input
	if len(payload) > 0 {
		zeros := 0
		for i := 0; i < len(payload); i++ {
			if payload[i] == 0 {
				zeros++
			}
		}
		nonZeros := len(payload) - zeros
		cost += uint64(zeros) * 4
		cost += uint64(nonZeros) * 68
	}

	return uint64(cost)
}

func (t *Transition) preCheck(msg *types.Transaction) (uint64, error) {
	// validate nonce
	nonce := t.state.GetNonce(msg.From)
	if nonce < msg.Nonce {
		return 0, fmt.Errorf("nonce is too low: %d < %d", nonce, msg.Nonce)
	} else if nonce > msg.Nonce {
		return 0, fmt.Errorf("nonce is too big: %d > %d", nonce, msg.Nonce)
	}

	// deduct the upfront max gas cost
	upfrontGasCost := new(big.Int).SetBytes(msg.GasPrice)
	upfrontGasCost = upfrontGasCost.Mul(upfrontGasCost, new(big.Int).SetUint64(msg.Gas))
	balance := t.state.GetBalance(msg.From)
	if balance.Cmp(upfrontGasCost) < 0 {
		return 0, ErrInsufficientBalanceForGas
	}
	t.state.SubBalance(msg.From, upfrontGasCost)

	// calculate gas available for the transaction
	intrinsicGas := t.transactionGasCost(msg)
	gasAvailable := msg.Gas - intrinsicGas

	return gasAvailable, nil
}

func (t *Transition) apply(msg *types.Transaction) (uint64, bool, error) {
	// check if there is enough gas in the pool
	if err := t.subGasPool(msg.Gas); err != nil {
		return 0, false, err
	}

	txn := t.state
	s := txn.Snapshot()

	gas, err := t.preCheck(msg)
	if err != nil {
		return 0, false, err
	}
	if gas > msg.Gas {
		return 0, false, errorVMOutOfGas
	}

	gasPrice := new(big.Int).SetBytes(msg.GetGasPrice())
	value := new(big.Int).SetBytes(msg.Value)

	// Set the specific transaction fields in the context
	t.ctx.GasPrice = types.BytesToHash(msg.GetGasPrice())
	t.ctx.Origin = msg.From

	var subErr error
	var gasLeft uint64

	if msg.IsContractCreation() {
		_, gasLeft, subErr = t.Create2(msg.From, msg.Input, value, gas)
	} else {
		txn.IncrNonce(msg.From)
		_, gasLeft, subErr = t.Call2(msg.From, *msg.To, msg.Input, value, gas)
	}
	if subErr != nil {
		if subErr == runtime.ErrNotEnoughFunds {
			txn.RevertToSnapshot(s)
			return 0, false, subErr
		}
	}

	gasUsed := msg.Gas - gasLeft
	refund := gasUsed / 2
	if refund > txn.GetRefund() {
		refund = txn.GetRefund()
	}

	gasLeft += refund
	gasUsed -= refund

	// refund the sender
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(gasLeft), gasPrice)
	txn.AddBalance(msg.From, remaining)

	// pay the coinbase
	coinbaseFee := new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), gasPrice)
	txn.AddBalance(t.ctx.Coinbase, coinbaseFee)

	// return gas to the pool
	t.addGasPool(gasLeft)

	return gasUsed, subErr != nil, nil
}

func (t *Transition) Create2(caller types.Address, code []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	address := crypto.CreateAddress(caller, t.state.GetNonce(caller))
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)
	return t.applyCreate(contract, t)
}

func (t *Transition) Call2(caller types.Address, to types.Address, input []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	c := runtime.NewContractCall(1, caller, caller, to, value, gas, t.state.GetCode(to), input)
	return t.applyCall(c, runtime.Call, t)
}

func (t *Transition) run(contract *runtime.Contract, host runtime.Host) ([]byte, uint64, error) {
	for _, r := range t.r.runtimes {
		if r.CanRun(contract, host, t.config) {
			return r.Run(contract, host, t.config)
		}
	}
	return nil, 0, fmt.Errorf("not found")
}

func (t *Transition) transfer(from, to types.Address, amount *big.Int) error {
	if amount == nil {
		return nil
	}

	balance := t.state.GetBalance(from)
	if balance.Cmp(amount) < 0 {
		return runtime.ErrNotEnoughFunds
	}

	t.state.SubBalance(from, amount)
	t.state.AddBalance(to, amount)
	return nil
}

func (t *Transition) applyCall(c *runtime.Contract, callType runtime.CallType, host runtime.Host) ([]byte, uint64, error) {
	if c.Depth > int(1024)+1 {
		return nil, c.Gas, runtime.ErrDepth
	}

	snapshot := t.state.Snapshot()
	t.state.TouchAccount(c.Address)

	if callType == runtime.Call {
		// Transfers only allowed on calls
		if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
			return nil, c.Gas, err
		}
	}

	ret, gas, err := t.run(c, host)
	if err != nil {
		t.state.RevertToSnapshot(snapshot)
	}
	return ret, gas, err
}

var emptyHash types.Hash

func (t *Transition) hasCodeOrNonce(addr types.Address) bool {
	nonce := t.state.GetNonce(addr)
	if nonce != 0 {
		return true
	}
	codeHash := t.state.GetCodeHash(addr)
	if codeHash != emptyCodeHashTwo && codeHash != emptyHash {
		return true
	}
	return false
}

func (t *Transition) applyCreate(msg *runtime.Contract, host runtime.Host) ([]byte, uint64, error) {
	if msg.Depth > int(1024)+1 {
		return nil, msg.Gas, runtime.ErrDepth
	}

	gas := msg.Gas

	// Incremene the nonce of the caller
	t.state.IncrNonce(msg.Caller)

	// Check if there if there is a collision and the address already exists
	if t.hasCodeOrNonce(msg.Address) {
		return nil, 0, runtime.ErrContractAddressCollision
	}

	// Take snapshot of the current state
	snapshot := t.state.Snapshot()

	if t.config.EIP158 {
		// Force the creation of the account
		t.state.CreateAccount(msg.Address)
		t.state.IncrNonce(msg.Address)
	}

	// Transfer the value
	if err := t.transfer(msg.Caller, msg.Address, msg.Value); err != nil {
		return nil, gas, runtime.ErrNotEnoughFunds
	}

	code, gas, err := t.run(msg, host)

	if err != nil {
		t.state.RevertToSnapshot(snapshot)
		return code, gas, err
	}

	if t.config.EIP158 && len(code) > spuriousDragonMaxCodeSize {
		// Contract size exceeds 'SpuriousDragon' size limit
		t.state.RevertToSnapshot(snapshot)
		return nil, 0, runtime.ErrMaxCodeSizeExceeded
	}

	gasCost := uint64(len(code)) * 200

	if gas < gasCost {
		// Out of gas creating the contract
		if t.config.Homestead {
			t.state.RevertToSnapshot(snapshot)
			gas = 0
		}
		return nil, gas, runtime.ErrCodeStoreOutOfGas
	}

	gas -= gasCost
	t.state.SetCode(msg.Address, code)

	return code, gas, nil
}

func (t *Transition) SetStorage(addr types.Address, key types.Hash, value types.Hash, discount bool) runtime.StorageStatus {
	return t.state.SetStorage(addr, key, value, discount)
}

func (t *Transition) GetTxContext() runtime.TxContext {
	return t.ctx
}

func (t *Transition) GetBlockHash(number int64) (res types.Hash) {
	return t.getHash(uint64(number))
}

func (t *Transition) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	t.state.EmitLog(addr, topics, data)
}

func (t *Transition) GetCodeSize(addr types.Address) int {
	return t.state.GetCodeSize(addr)
}

func (t *Transition) GetCodeHash(addr types.Address) (res types.Hash) {
	return t.state.GetCodeHash(addr)
}

func (t *Transition) GetCode(addr types.Address) []byte {
	return t.state.GetCode(addr)
}

func (t *Transition) GetBalance(addr types.Address) *big.Int {
	return t.state.GetBalance(addr)
}

func (t *Transition) GetStorage(addr types.Address, key types.Hash) types.Hash {
	return t.state.GetState(addr, key)
}

func (t *Transition) AccountExists(addr types.Address) bool {
	return t.state.Exist(addr)
}

func (t *Transition) Empty(addr types.Address) bool {
	return t.state.Empty(addr)
}

func (t *Transition) GetNonce(addr types.Address) uint64 {
	return t.state.GetNonce(addr)
}

func (t *Transition) Selfdestruct(addr types.Address, beneficiary types.Address) {
	if !t.state.HasSuicided(addr) {
		t.state.AddRefund(24000)
	}
	t.state.AddBalance(beneficiary, t.state.GetBalance(addr))
	t.state.Suicide(addr)
}

func (t *Transition) Callx(c *runtime.Contract, h runtime.Host) ([]byte, uint64, error) {
	if c.Type == runtime.Create {
		return t.applyCreate(c, h)
	}
	return t.applyCall(c, c.Type, h)
}
