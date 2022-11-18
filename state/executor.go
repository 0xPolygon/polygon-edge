package state

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	spuriousDragonMaxCodeSize = 24576

	TxGas                 uint64 = 21000 // Per transaction not creating a contract
	TxGasContractCreation uint64 = 53000 // Per transaction that creates a contract
)

var emptyCodeHashTwo = types.BytesToHash(crypto.Keccak256(nil))

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) types.Hash

type GetHashByNumberHelper = func(*types.Header) GetHashByNumber

// Executor is the main entity
type Executor struct {
	logger  hclog.Logger
	config  *chain.Params
	state   State
	GetHash GetHashByNumberHelper

	PostHook func(txn *Transition)
}

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s State, logger hclog.Logger) *Executor {
	return &Executor{
		logger: logger,
		config: config,
		state:  s,
	}
}

func (e *Executor) WriteGenesis(alloc map[types.Address]*chain.GenesisAccount) types.Hash {
	snap := e.state.NewSnapshot()
	txn := NewTxn(snap)

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

	objs := txn.Commit(false)
	_, root := snap.Commit(objs)

	return types.BytesToHash(root)
}

type BlockResult struct {
	Root     types.Hash
	Receipts []*types.Receipt
	TotalGas uint64
}

// ProcessBlock already does all the handling of the whole process
func (e *Executor) ProcessBlock(
	parentRoot types.Hash,
	block *types.Block,
	blockCreator types.Address,
) (*Transition, error) {
	txn, err := e.BeginTxn(parentRoot, block.Header, blockCreator)
	if err != nil {
		return nil, err
	}

	for _, t := range block.Transactions {
		if t.ExceedsBlockGasLimit(block.Header.GasLimit) {
			if err := txn.WriteFailedReceipt(t); err != nil {
				return nil, err
			}

			continue
		}

		if err := txn.Write(t); err != nil {
			return nil, err
		}
	}

	return txn, nil
}

// StateAt returns snapshot at given root
func (e *Executor) State() State {
	return e.state
}

// StateAt returns snapshot at given root
func (e *Executor) StateAt(root types.Hash) (Snapshot, error) {
	return e.state.NewSnapshotAt(root)
}

// GetForksInTime returns the active forks at the given block height
func (e *Executor) GetForksInTime(blockNumber uint64) chain.ForksInTime {
	return e.config.Forks.At(blockNumber)
}

func (e *Executor) BeginTxn(
	parentRoot types.Hash,
	header *types.Header,
	coinbaseReceiver types.Address,
) (*Transition, error) {
	forkConfig := e.config.Forks.At(header.Number)

	auxSnap2, err := e.state.NewSnapshotAt(parentRoot)
	if err != nil {
		return nil, err
	}

	newTxn := NewTxn(auxSnap2)

	txCtx := runtime.TxContext{
		Coinbase:   coinbaseReceiver,
		Timestamp:  int64(header.Timestamp),
		Number:     int64(header.Number),
		Difficulty: types.BytesToHash(new(big.Int).SetUint64(header.Difficulty).Bytes()),
		GasLimit:   int64(header.GasLimit),
		ChainID:    int64(e.config.ChainID),
	}

	txn := &Transition{
		logger:   e.logger,
		ctx:      txCtx,
		state:    newTxn,
		snap:     auxSnap2,
		getHash:  e.GetHash(header),
		auxState: e.state,
		config:   forkConfig,
		gasPool:  uint64(txCtx.GasLimit),

		receipts: []*types.Receipt{},
		totalGas: 0,

		evm:         evm.NewEVM(),
		precompiles: precompiled.NewPrecompiled(),
		PostHook:    e.PostHook,
	}

	return txn, nil
}

type Transition struct {
	logger hclog.Logger

	// dummy
	auxState State
	snap     Snapshot

	config  chain.ForksInTime
	state   *Txn
	getHash GetHashByNumber
	ctx     runtime.TxContext
	gasPool uint64

	// result
	receipts []*types.Receipt
	totalGas uint64

	PostHook func(t *Transition)

	// runtimes
	evm         *evm.EVM
	precompiles *precompiled.Precompiled
}

func NewTransition(config chain.ForksInTime, snap Snapshot, radix *Txn) *Transition {
	return &Transition{
		config:      config,
		state:       radix,
		snap:        snap,
		evm:         evm.NewEVM(),
		precompiles: precompiled.NewPrecompiled(),
	}
}

func (t *Transition) TotalGas() uint64 {
	return t.totalGas
}

func (t *Transition) Receipts() []*types.Receipt {
	return t.receipts
}

var emptyFrom = types.Address{}

func (t *Transition) WriteFailedReceipt(txn *types.Transaction) error {
	signer := crypto.NewSigner(t.config, uint64(t.ctx.ChainID))

	if txn.From == emptyFrom {
		// Decrypt the from address
		from, err := signer.Sender(txn)
		if err != nil {
			return NewTransitionApplicationError(err, false)
		}

		txn.From = from
	}

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TxHash:            txn.Hash,
		Logs:              t.state.Logs(),
	}

	receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
	receipt.SetStatus(types.ReceiptFailed)
	t.receipts = append(t.receipts, receipt)

	if txn.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(txn.From, txn.Nonce).Ptr()
	}

	return nil
}

// Write writes another transaction to the executor
func (t *Transition) Write(txn *types.Transaction) error {
	signer := crypto.NewSigner(t.config, uint64(t.ctx.ChainID))

	var err error
	if txn.From == emptyFrom {
		// Decrypt the from address
		txn.From, err = signer.Sender(txn)
		if err != nil {
			return NewTransitionApplicationError(err, false)
		}
	}

	// Make a local copy and apply the transaction
	msg := txn.Copy()

	result, e := t.Apply(msg)
	if e != nil {
		t.logger.Error("failed to apply tx", "err", e)

		return e
	}

	t.totalGas += result.GasUsed

	logs := t.state.Logs()

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TxHash:            txn.Hash,
		GasUsed:           result.GasUsed,
	}

	// The suicided accounts are set as deleted for the next iteration
	t.state.CleanDeleteObjects(true)

	if result.Failed() {
		receipt.SetStatus(types.ReceiptFailed)
	} else {
		receipt.SetStatus(types.ReceiptSuccess)
	}

	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(msg.From, txn.Nonce).Ptr()
	}

	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = logs
	receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
	t.receipts = append(t.receipts, receipt)

	return nil
}

// Commit commits the final result
func (t *Transition) Commit() (Snapshot, types.Hash) {
	objs := t.state.Commit(t.config.EIP155)
	s2, root := t.snap.Commit(objs)

	return s2, types.BytesToHash(root)
}

func (t *Transition) subGasPool(amount uint64) error {
	if t.gasPool < amount {
		return ErrBlockLimitReached
	}

	t.gasPool -= amount

	return nil
}

func (t *Transition) addGasPool(amount uint64) {
	t.gasPool += amount
}

func (t *Transition) Txn() *Txn {
	return t.state
}

// Apply applies a new transaction
func (t *Transition) Apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	s := t.state.Snapshot()

	result, err := t.apply(msg)
	if err != nil {
		t.state.RevertToSnapshot(s)
	}

	if t.PostHook != nil {
		t.PostHook(t)
	}

	return result, err
}

// ContextPtr returns reference of context
// This method is called only by test
func (t *Transition) ContextPtr() *runtime.TxContext {
	return &t.ctx
}

func (t *Transition) subGasLimitPrice(msg *types.Transaction) error {
	// deduct the upfront max gas cost
	upfrontGasCost := new(big.Int).Set(msg.GasPrice)
	upfrontGasCost.Mul(upfrontGasCost, new(big.Int).SetUint64(msg.Gas))

	if err := t.state.SubBalance(msg.From, upfrontGasCost); err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return ErrNotEnoughFundsForGas
		}

		return err
	}

	return nil
}

func (t *Transition) nonceCheck(msg *types.Transaction) error {
	nonce := t.state.GetNonce(msg.From)

	if nonce != msg.Nonce {
		return ErrNonceIncorrect
	}

	return nil
}

// errors that can originate in the consensus rules checks of the apply method below
// surfacing of these errors reject the transaction thus not including it in the block

var (
	ErrNonceIncorrect        = fmt.Errorf("incorrect nonce")
	ErrNotEnoughFundsForGas  = fmt.Errorf("not enough funds to cover gas costs")
	ErrBlockLimitReached     = fmt.Errorf("gas limit reached in the pool")
	ErrIntrinsicGasOverflow  = fmt.Errorf("overflow in intrinsic gas calculation")
	ErrNotEnoughIntrinsicGas = fmt.Errorf("not enough gas supplied for intrinsic gas costs")
	ErrNotEnoughFunds        = fmt.Errorf("not enough funds for transfer with given value")
)

type TransitionApplicationError struct {
	Err           error
	IsRecoverable bool // Should the transaction be discarded, or put back in the queue.
}

func (e *TransitionApplicationError) Error() string {
	return e.Err.Error()
}

func NewTransitionApplicationError(err error, isRecoverable bool) *TransitionApplicationError {
	return &TransitionApplicationError{
		Err:           err,
		IsRecoverable: isRecoverable,
	}
}

type GasLimitReachedTransitionApplicationError struct {
	TransitionApplicationError
}

func NewGasLimitReachedTransitionApplicationError(err error) *GasLimitReachedTransitionApplicationError {
	return &GasLimitReachedTransitionApplicationError{
		*NewTransitionApplicationError(err, true),
	}
}

func (t *Transition) apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. there is no overflow when calculating intrinsic gas
	// 5. the purchased gas is enough to cover intrinsic usage
	// 6. caller has enough balance to cover asset transfer for **topmost** call
	txn := t.state

	// 1. the nonce of the message caller is correct
	if err := t.nonceCheck(msg); err != nil {
		return nil, NewTransitionApplicationError(err, true)
	}

	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	if err := t.subGasLimitPrice(msg); err != nil {
		return nil, NewTransitionApplicationError(err, true)
	}

	// 3. the amount of gas required is available in the block
	if err := t.subGasPool(msg.Gas); err != nil {
		return nil, NewGasLimitReachedTransitionApplicationError(err)
	}

	if t.ctx.Tracer != nil {
		t.ctx.Tracer.TxStart(msg.Gas)
	}

	// 4. there is no overflow when calculating intrinsic gas
	intrinsicGasCost, err := TransactionGasCost(msg, t.config.Homestead, t.config.Istanbul)
	if err != nil {
		return nil, NewTransitionApplicationError(err, false)
	}

	// 5. the purchased gas is enough to cover intrinsic usage
	gasLeft := msg.Gas - intrinsicGasCost
	// Because we are working with unsigned integers for gas, the `>` operator is used instead of the more intuitive `<`
	if gasLeft > msg.Gas {
		return nil, NewTransitionApplicationError(ErrNotEnoughIntrinsicGas, false)
	}

	// 6. caller has enough balance to cover asset transfer for **topmost** call
	if balance := txn.GetBalance(msg.From); balance.Cmp(msg.Value) < 0 {
		return nil, NewTransitionApplicationError(ErrNotEnoughFunds, true)
	}

	gasPrice := new(big.Int).Set(msg.GasPrice)
	value := new(big.Int).Set(msg.Value)

	// Set the specific transaction fields in the context
	t.ctx.GasPrice = types.BytesToHash(gasPrice.Bytes())
	t.ctx.Origin = msg.From

	var result *runtime.ExecutionResult
	if msg.IsContractCreation() {
		result = t.Create2(msg.From, msg.Input, value, gasLeft)
	} else {
		txn.IncrNonce(msg.From)
		result = t.Call2(msg.From, *msg.To, msg.Input, value, gasLeft)
	}

	refund := txn.GetRefund()
	result.UpdateGasUsed(msg.Gas, refund)

	if t.ctx.Tracer != nil {
		t.ctx.Tracer.TxEnd(result.GasLeft)
	}

	// refund the sender
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(result.GasLeft), gasPrice)
	txn.AddBalance(msg.From, remaining)

	// pay the coinbase
	coinbaseFee := new(big.Int).Mul(new(big.Int).SetUint64(result.GasUsed), gasPrice)
	txn.AddBalance(t.ctx.Coinbase, coinbaseFee)

	// return gas to the pool
	t.addGasPool(result.GasLeft)

	return result, nil
}

func (t *Transition) Create2(
	caller types.Address,
	code []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	address := crypto.CreateAddress(caller, t.state.GetNonce(caller))
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)

	return t.applyCreate(contract, t)
}

func (t *Transition) Call2(
	caller types.Address,
	to types.Address,
	input []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	c := runtime.NewContractCall(1, caller, caller, to, value, gas, t.state.GetCode(to), input)

	return t.applyCall(c, runtime.Call, t)
}

func (t *Transition) run(contract *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {
	// check the precompiles
	if t.precompiles.CanRun(contract, host, &t.config) {
		return t.precompiles.Run(contract, host, &t.config)
	}
	// check the evm
	if t.evm.CanRun(contract, host, &t.config) {
		return t.evm.Run(contract, host, &t.config)
	}

	return &runtime.ExecutionResult{
		Err: fmt.Errorf("runtime not found"),
	}
}

func (t *Transition) transfer(from, to types.Address, amount *big.Int) error {
	if amount == nil {
		return nil
	}

	if err := t.state.SubBalance(from, amount); err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return runtime.ErrInsufficientBalance
		}

		return err
	}

	t.state.AddBalance(to, amount)

	return nil
}

func (t *Transition) applyCall(
	c *runtime.Contract,
	callType runtime.CallType,
	host runtime.Host,
) *runtime.ExecutionResult {
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

	var result *runtime.ExecutionResult

	t.captureCallStart(c, callType)

	result = t.run(c, host)
	if result.Failed() {
		t.state.RevertToSnapshot(snapshot)
	}

	t.captureCallEnd(c, result)

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
			Err:     err,
		}
	}

	var result *runtime.ExecutionResult

	t.captureCallStart(c, evm.CREATE)

	defer func() {
		// pass result to be set later
		t.captureCallEnd(c, result)
	}()

	result = t.run(c, host)
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

func (t *Transition) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
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

// SetAccountDirectly sets an account to the given address
// NOTE: SetAccountDirectly changes the world state without a transaction
func (t *Transition) SetAccountDirectly(addr types.Address, account *chain.GenesisAccount) error {
	if t.AccountExists(addr) {
		return fmt.Errorf("can't add account to %+v because an account exists already", addr)
	}

	t.state.SetCode(addr, account.Code)

	for key, value := range account.Storage {
		t.state.SetStorage(addr, key, value, &t.config)
	}

	t.state.SetBalance(addr, account.Balance)
	t.state.SetNonce(addr, account.Nonce)

	return nil
}

// SetCodeDirectly sets new code into the account with the specified address
// NOTE: SetCodeDirectly changes the world state without a transaction
func (t *Transition) SetCodeDirectly(addr types.Address, code []byte) error {
	if !t.AccountExists(addr) {
		return fmt.Errorf("account doesn't exist at %s", addr)
	}

	t.state.SetCode(addr, code)

	return nil
}

// SetTracer sets tracer to the context in order to enable it
func (t *Transition) SetTracer(tracer tracer.Tracer) {
	t.ctx.Tracer = tracer
}

// GetTracer returns a tracer in context
func (t *Transition) GetTracer() runtime.VMTracer {
	return t.ctx.Tracer
}

func (t *Transition) GetRefund() uint64 {
	return t.state.GetRefund()
}

func TransactionGasCost(msg *types.Transaction, isHomestead, isIstanbul bool) (uint64, error) {
	cost := uint64(0)

	// Contract creation is only paid on the homestead fork
	if msg.IsContractCreation() && isHomestead {
		cost += TxGasContractCreation
	} else {
		cost += TxGas
	}

	payload := msg.Input
	if len(payload) > 0 {
		zeros := uint64(0)

		for i := 0; i < len(payload); i++ {
			if payload[i] == 0 {
				zeros++
			}
		}

		nonZeros := uint64(len(payload)) - zeros
		nonZeroCost := uint64(68)

		if isIstanbul {
			nonZeroCost = 16
		}

		if (math.MaxUint64-cost)/nonZeroCost < nonZeros {
			return 0, ErrIntrinsicGasOverflow
		}

		cost += nonZeros * nonZeroCost

		if (math.MaxUint64-cost)/4 < zeros {
			return 0, ErrIntrinsicGasOverflow
		}

		cost += zeros * 4
	}

	return cost, nil
}

// captureCallStart calls CallStart in Tracer if context has the tracer
func (t *Transition) captureCallStart(c *runtime.Contract, callType runtime.CallType) {
	if t.ctx.Tracer == nil {
		return
	}

	t.ctx.Tracer.CallStart(
		c.Depth,
		c.Caller,
		c.Address,
		int(callType),
		c.Gas,
		c.Value,
		c.Input,
	)
}

// captureCallEnd calls CallEnd in Tracer if context has the tracer
func (t *Transition) captureCallEnd(c *runtime.Contract, result *runtime.ExecutionResult) {
	if t.ctx.Tracer == nil {
		return
	}

	t.ctx.Tracer.CallEnd(
		c.Depth,
		result.ReturnValue,
		result.Err,
	)
}
