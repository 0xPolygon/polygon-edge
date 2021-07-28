package state

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/types"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/state/runtime"
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

type GetHashByNumberHelper = func(*types.Header) GetHashByNumber

// Executor is the main entity
type Executor struct {
	config   *chain.Params
	runtimes []runtime.Runtime
	state    State
	GetHash  GetHashByNumberHelper

	PostHook func(txn *Transition)
}

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s State) *Executor {
	return &Executor{
		config:   config,
		runtimes: []runtime.Runtime{},
		state:    s,
	}
}

func (e *Executor) WriteGenesis(alloc map[types.Address]*chain.GenesisAccount) types.Hash {
	snap := e.state.NewSnapshot()
	txn := NewTxn(e.state, snap)

	for addr, account := range alloc {
		if account.Balance != nil {
			txn.AddBalance(addr, account.Balance)
		}
		if account.Nonce != 0 {
			txn.SetNonce(addr, account.Nonce)
		}
		if len(account.Code) != 0 {
			txn.SetCode(addr, account.Code)
		}
		for key, value := range account.Storage {
			txn.SetState(addr, key, value)
		}
	}

	_, root := txn.Commit(false)
	return types.BytesToHash(root)
}

// SetRuntime adds a runtime to the runtime set
func (e *Executor) SetRuntime(r runtime.Runtime) {
	e.runtimes = append(e.runtimes, r)
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// ProcessBlock already does all the handling of the whole process, TODO
func (e *Executor) ProcessBlock(parentRoot types.Hash, block *types.Block, blockCreator types.Address) (*BlockResult, error) {
	txn, err := e.BeginTxn(parentRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	txn.block = block
	for _, t := range block.Transactions {
		if err := txn.Write(t); err != nil {
			return nil, err
		}
	}
	_, root := txn.Commit()

	res := &BlockResult{
		Root:     root,
		Receipts: txn.Receipts(),
		TotalGas: txn.TotalGas(),
	}
	return res, nil
}

// StateAt returns snapshot at given root
func (e *Executor) State() State {
	return e.state
}

// StateAt returns snapshot at given root
func (e *Executor) StateAt(root types.Hash) (Snapshot, error) {
	return e.state.NewSnapshotAt(root)
}

func (e *Executor) BeginTxn(parentRoot types.Hash, header *types.Header, coinbaseReceiver types.Address) (*Transition, error) {
	config := e.config.Forks.At(header.Number)

	auxSnap2, err := e.state.NewSnapshotAt(parentRoot)
	if err != nil {
		return nil, err
	}

	newTxn := NewTxn(e.state, auxSnap2)

	env2 := runtime.TxContext{
		Coinbase:   coinbaseReceiver,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
		ChainID:    int64(e.config.ChainID),
	}

	txn := &Transition{
		r:        e,
		ctx:      env2,
		state:    newTxn,
		getHash:  e.GetHash(header),
		auxState: e.state,
		config:   config,
		gasPool:  uint64(env2.GasLimit),

		receipts: []*types.Receipt{},
		totalGas: 0,
	}
	return txn, nil
}

type Transition struct {
	// dummy
	auxState State

	// the current block being processed
	block *types.Block

	r       *Executor
	config  chain.ForksInTime
	state   *Txn
	getHash GetHashByNumber
	ctx     runtime.TxContext
	gasPool uint64

	// result
	receipts []*types.Receipt
	totalGas uint64

	// The return value for the contract execution
	returnValue []byte
}

func (t *Transition) ReturnValue() []byte {
	return t.returnValue
}

func (t *Transition) TotalGas() uint64 {
	return t.totalGas
}

func (t *Transition) Receipts() []*types.Receipt {
	return t.receipts
}

var emptyFrom = types.Address{}

// Write writes another transaction to the executor
func (t *Transition) Write(txn *types.Transaction) error {
	signer := crypto.NewSigner(t.config, uint64(t.r.config.ChainID))

	var err error
	if txn.From == emptyFrom {
		// Decrypt the from address
		txn.From, err = signer.Sender(txn)
		if err != nil {
			return err
		}
	}

	// Make a local copy and apply the transaction
	msg := txn.Copy()

	gasUsed, failed, err := t.Apply(msg)
	if err != nil {
		fmt.Printf("Apply err: %v", err)
	}
	t.totalGas += gasUsed

	logs := t.state.Logs()

	var root []byte

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TxHash:            txn.Hash,
		GasUsed:           gasUsed,
	}

	if t.config.Byzantium {
		// The suicided accounts are set as deleted for the next iteration
		t.state.CleanDeleteObjects(true)

		if failed {
			receipt.SetStatus(types.ReceiptFailed)
		} else {
			receipt.SetStatus(types.ReceiptSuccess)
		}

	} else {
		ss, aux := t.state.Commit(t.config.EIP155)
		t.state = NewTxn(t.auxState, ss)
		root = aux
		receipt.Root = types.BytesToHash(root)
	}

	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(msg.From, txn.Nonce)
	}

	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = logs
	receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
	t.receipts = append(t.receipts, receipt)

	return nil
}

// Commit commits the final result
func (t *Transition) Commit() (Snapshot, types.Hash) {
	s2, root := t.state.Commit(t.config.EIP155)

	return s2, types.BytesToHash(root)
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

func (t *Transition) GetTxnHash() types.Hash {
	return t.block.Hash()
}

// Apply applies a new transaction
func (t *Transition) Apply(msg *types.Transaction) (gasUsed uint64, failed bool, err error) {
	s := t.state.Snapshot()
	result, err := t.apply(msg)
	if err != nil {
		t.state.RevertToSnapshot(s)
	}

	if t.r.PostHook != nil {
		t.r.PostHook(t)
	}

	t.returnValue = result.ReturnValue

	gasUsed = msg.Gas - result.GasLeft
	failed = result.Failed() || err != nil

	return gasUsed, failed, err
}

// ContextPtr returns reference of context
// This method is called only by test
func (t *Transition) ContextPtr() *runtime.TxContext {
	return &t.ctx
}

//@TODO: We don't have uint overflow check here
func (t *Transition) intrinsicGasCost(msg *types.Transaction) uint64 {
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

		nonZeroCost := uint64(68)
		if t.config.Istanbul {
			nonZeroCost = 16
		}
		cost += uint64(nonZeros) * nonZeroCost
	}

	return cost
}

func (t *Transition) subGasLimitPrice(msg *types.Transaction) error {
	// deduct the upfront max gas cost
	upfrontGasCost := new(big.Int).Set(msg.GasPrice)
	upfrontGasCost.Mul(upfrontGasCost, new(big.Int).SetUint64(msg.Gas))

	return t.state.SubBalance(msg.From, upfrontGasCost)
}

func (t *Transition) nonceCheck(msg *types.Transaction) error {
	nonce := t.state.GetNonce(msg.From)

	if nonce < msg.Nonce {
		return fmt.Errorf("nonce is too big: %d < %d", nonce, msg.Nonce)
	} else if nonce > msg.Nonce {
		return fmt.Errorf("nonce is too low: %d > %d", nonce, msg.Nonce)
	}

	return nil
}

func (t *Transition) apply(msg *types.Transaction) (result *runtime.ExecutionResult, err error) {
	result = &runtime.ExecutionResult{}

	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. the purchased gas is enough to cover intrinsic usage
	// 5. there is no overflow when calculating intrinsic gas
	// 6. caller has enough balance to cover asset transfer for **topmost** call

	txn := t.state

	// 1. the nonce of the message caller is correct
	if err := t.nonceCheck(msg); err != nil {
		return result, err
	}

	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	if err := t.subGasLimitPrice(msg); err != nil {
		return result, err
	}

	// 3. the amount of gas required is available in the block
	if err := t.subGasPool(msg.Gas); err != nil {
		return result, err
	}

	// 4. the purchased gas is enough to cover intrinsic usage
	// @TODO: We should check clause 5 (uint overflow) inside intrinsicGasCost()
	gasAvailable := msg.Gas - t.intrinsicGasCost(msg)
	// Because we are working with unsigned integers for gas, the `>` operator is used instead of the more intuitive `<`
	if gasAvailable > msg.Gas {
		return result, err
	}

	// 6. caller has enough balance to cover asset transfer for **topmost** call
	if balance := txn.GetBalance(msg.From); balance.Cmp(msg.Value) < 0 {
		return result, fmt.Errorf("not enough funds for transfer with given value")
	}

	gasPrice := new(big.Int).Set(msg.GasPrice)
	value := new(big.Int).Set(msg.Value)

	// Set the specific transaction fields in the context
	t.ctx.GasPrice = types.BytesToHash(gasPrice.Bytes())
	t.ctx.Origin = msg.From

	if msg.IsContractCreation() {
		result = t.Create2(msg.From, msg.Input, value, gasAvailable)
	} else {
		txn.IncrNonce(msg.From)
		result = t.Call2(msg.From, *msg.To, msg.Input, value, gasAvailable)
	}

	gasUsed := msg.Gas - result.GasLeft
	refund := gasUsed / 2

	// Refund can go up to half the gas used
	if refund > txn.GetRefund() {
		refund = txn.GetRefund()
	}

	result.GasLeft += refund
	gasUsed -= refund

	// refund the sender
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(result.GasLeft), gasPrice)
	txn.AddBalance(msg.From, remaining)

	// pay the coinbase
	coinbaseFee := new(big.Int).Mul(new(big.Int).SetUint64(gasUsed), gasPrice)
	txn.AddBalance(t.ctx.Coinbase, coinbaseFee)

	// return gas to the pool
	t.addGasPool(result.GasLeft)

	return result, nil
}

func (t *Transition) Create2(caller types.Address, code []byte, value *big.Int, gas uint64) *runtime.ExecutionResult {
	address := crypto.CreateAddress(caller, t.state.GetNonce(caller))
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)
	return t.applyCreate(contract, t)
}

func (t *Transition) Call2(caller types.Address, to types.Address, input []byte, value *big.Int, gas uint64) *runtime.ExecutionResult {
	c := runtime.NewContractCall(1, caller, caller, to, value, gas, t.state.GetCode(to), input)
	return t.applyCall(c, runtime.Call, t)
}

func (t *Transition) run(contract *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {
	for _, r := range t.r.runtimes {
		if r.CanRun(contract, host, &t.config) {
			return r.Run(contract, host, &t.config)
		}
	}

	return &runtime.ExecutionResult{
		Err: fmt.Errorf("not found"),
	}
}

func (t *Transition) transfer(from, to types.Address, amount *big.Int) error {
	if amount == nil {
		return nil
	}

	if err := t.state.SubBalance(from, amount); err != nil {
		return err
	}

	t.state.AddBalance(to, amount)
	return nil
}

func (t *Transition) applyCall(c *runtime.Contract, callType runtime.CallType, host runtime.Host) *runtime.ExecutionResult {
	if c.Depth > int(1024)+1 {
		return &runtime.ExecutionResult{
			GasLeft: c.Gas,
			Err:     runtime.ErrDepth,
		}
	}

	snapshot := t.state.Snapshot()
	t.state.TouchAccount(c.Address)

	if callType == runtime.Call {
		// Transfers only allowed on calls
		if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
			return &runtime.ExecutionResult{
				GasLeft: c.Gas,
				Err:     err,
			}
		}
	}

	result := t.run(c, host)
	if result.Failed() {
		t.state.RevertToSnapshot(snapshot)
	}

	return result
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

func (t *Transition) applyCreate(c *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {
	gasLimit := c.Gas

	if c.Depth > int(1024)+1 {
		return &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     runtime.ErrDepth,
		}
	}

	// Increment the nonce of the caller
	t.state.IncrNonce(c.Caller)

	// Check if there if there is a collision and the address already exists
	if t.hasCodeOrNonce(c.Address) {
		return &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrContractAddressCollision,
		}
	}

	// Take snapshot of the current state
	snapshot := t.state.Snapshot()

	if t.config.EIP158 {
		// Force the creation of the account
		t.state.CreateAccount(c.Address)
		t.state.IncrNonce(c.Address)
	}

	// Transfer the value
	if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
		return &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     runtime.ErrNotEnoughFunds,
		}
	}

	result := t.run(c, host)

	if result.Failed() {
		t.state.RevertToSnapshot(snapshot)
		return result
	}

	if t.config.EIP158 && len(result.ReturnValue) > spuriousDragonMaxCodeSize {
		// Contract size exceeds 'SpuriousDragon' size limit
		t.state.RevertToSnapshot(snapshot)
		return &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrMaxCodeSizeExceeded,
		}
	}

	gasCost := uint64(len(result.ReturnValue)) * 200

	if result.GasLeft < gasCost {
		result.Err = runtime.ErrCodeStoreOutOfGas
		result.ReturnValue = nil

		// Out of gas creating the contract
		if t.config.Homestead {
			t.state.RevertToSnapshot(snapshot)
			result.GasLeft = 0
		}

		return result
	}

	result.GasLeft -= gasCost
	t.state.SetCode(c.Address, result.ReturnValue)

	return result
}

func (t *Transition) SetStorage(addr types.Address, key types.Hash, value types.Hash, config *chain.ForksInTime) runtime.StorageStatus {
	return t.state.SetStorage(addr, key, value, config)
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

func (t *Transition) Callx(c *runtime.Contract, h runtime.Host) *runtime.ExecutionResult {
	if c.Type == runtime.Create {
		return t.applyCreate(c, h)
	}
	return t.applyCall(c, c.Type, h)
}
