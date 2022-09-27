package state

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/hashicorp/go-hclog"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/state/runtime/precompiled"
	"github.com/0xPolygon/polygon-edge/types"
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
	logger   hclog.Logger
	config   *chain.Params
	runtimes []runtime.Runtime
	state    State
	GetHash  GetHashByNumberHelper

	PostHook PostHook
}

type PostHook func(txn *Transition)

// NewExecutor creates a new executor
func NewExecutor(config *chain.Params, s State, logger hclog.Logger) *Executor {
	return &Executor{
		logger:   logger,
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

	objs := txn.Commit(false)
	_, root := snap.Commit(objs)

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
		var receipt *types.Receipt

		if t.ExceedsBlockGasLimit(block.Header.GasLimit) {
			receipt, err = txn.WriteFailedReceipt(t)
			if err != nil {
				return nil, err
			}
		} else {
			receipt, err = txn.Write(t)
			if err != nil {
				return nil, err
			}
		}

		txn.receipts = append(txn.receipts, receipt)
	}

	return txn, nil
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

	txn := NewTransition1()
	txn.Ctx = env2
	txn.Config = config
	txn.State = newTxn
	txn.GetHash = e.GetHash(header)
	txn.GasPool = uint64(env2.GasLimit)

	return &Transition{receipts: []*types.Receipt{}, writeSnapshot: auxSnap2, hook: e.PostHook, Transition1: txn}, nil
}

type Transition struct {
	*Transition1

	writeSnapshot Snapshot

	hook PostHook

	receipts []*types.Receipt
}

// Apply applies a new transaction
func (t *Transition) Apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	result, err := t.Transition1.Apply(msg)

	if t.hook != nil {
		t.hook(t)
	}

	return result, err
}

func (t *Transition) Commit() (Snapshot, types.Hash) {
	s2, root := t.writeSnapshot.Commit(t.Transition1.Commit2())
	return s2, types.BytesToHash(root)
}

func (t *Transition) Receipts() []*types.Receipt {
	return t.receipts
}

type Transition1 struct {
	logger hclog.Logger

	Config  chain.ForksInTime
	State   *Txn
	GetHash GetHashByNumber
	Ctx     runtime.TxContext
	GasPool uint64

	// runtimes
	evm         *evm.EVM
	precompiled *precompiled.Precompiled

	// result
	totalGas uint64
}

func (t *Transition1) SetLogger(logger hclog.Logger) {
	t.logger = logger
}

func NewTransition1() *Transition1 {
	t := &Transition1{
		logger:      hclog.NewNullLogger(),
		evm:         evm.NewEVM(),
		precompiled: precompiled.NewPrecompiled(),
	}
	return t
}

func (t *Transition1) TotalGas() uint64 {
	return t.totalGas
}

var emptyFrom = types.Address{}

func (t *Transition1) WriteFailedReceipt(txn *types.Transaction) (*types.Receipt, error) {
	signer := crypto.NewSigner(t.Config, uint64(t.Ctx.ChainID))

	if txn.From == emptyFrom {
		// Decrypt the from address
		from, err := signer.Sender(txn)
		if err != nil {
			return nil, NewTransitionApplicationError(err, false)
		}

		txn.From = from
	}

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TxHash:            txn.Hash,
		Logs:              t.State.Logs(),
	}

	receipt.LogsBloom = types.CreateBloom([]*types.Receipt{receipt})
	receipt.SetStatus(types.ReceiptFailed)
	//t.receipts = append(t.receipts, receipt)

	if txn.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(txn.From, txn.Nonce).Ptr()
	}

	return receipt, nil
}

// Write writes another transaction to the executor
func (t *Transition1) Write(txn *types.Transaction) (*types.Receipt, error) {
	signer := crypto.NewSigner(t.Config, uint64(t.Ctx.ChainID))

	var err error
	if txn.From == emptyFrom {
		// Decrypt the from address
		txn.From, err = signer.Sender(txn)
		if err != nil {
			return nil, NewTransitionApplicationError(err, false)
		}
	}

	// Make a local copy and apply the transaction
	msg := txn.Copy()

	result, err := t.Apply(msg)
	if err != nil {
		t.logger.Error("failed to apply tx", "err", err)

		return nil, err
	}

	t.totalGas += result.GasUsed

	logs := t.State.Logs()

	receipt := &types.Receipt{
		CumulativeGasUsed: t.totalGas,
		TxHash:            txn.Hash,
		GasUsed:           result.GasUsed,
	}

	// The suicided accounts are set as deleted for the next iteration
	t.State.CleanDeleteObjects(true)

	// TODO: Remove for now the pre-byzantium hard fork
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
	//t.receipts = append(t.receipts, receipt)

	return receipt, nil
}

func (t *Transition1) Commit2() []*Object {
	return t.State.Commit(t.Config.EIP155)
}

// Commit commits the final result
func (t *Transition1) Commit() (Snapshot, types.Hash) {
	objs := t.State.Commit(t.Config.EIP155)
	s2, root := t.State.snapshot.Commit(objs)

	return s2, types.BytesToHash(root)
}

func (t *Transition1) subGasPool(amount uint64) error {
	if t.GasPool < amount {
		return ErrBlockLimitReached
	}

	t.GasPool -= amount

	return nil
}

func (t *Transition1) addGasPool(amount uint64) {
	t.GasPool += amount
}

func (t *Transition1) SetTxn(txn *Txn) {
	t.State = txn
}

func (t *Transition1) Txn() *Txn {
	return t.State
}

// Apply applies a new transaction
func (t *Transition1) Apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	s := t.State.Snapshot()
	result, err := t.apply(msg)

	if err != nil {
		t.State.RevertToSnapshot(s)
	}

	return result, err
}

// ContextPtr returns reference of context
// This method is called only by test
func (t *Transition1) ContextPtr() *runtime.TxContext {
	return &t.Ctx
}

func (t *Transition1) subGasLimitPrice(msg *types.Transaction) error {
	// deduct the upfront max gas cost
	upfrontGasCost := new(big.Int).Set(msg.GasPrice)
	upfrontGasCost.Mul(upfrontGasCost, new(big.Int).SetUint64(msg.Gas))

	if err := t.State.SubBalance(msg.From, upfrontGasCost); err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return ErrNotEnoughFundsForGas
		}

		return err
	}

	return nil
}

func (t *Transition1) nonceCheck(msg *types.Transaction) error {
	nonce := t.State.GetNonce(msg.From)

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

func (t *Transition1) apply(msg *types.Transaction) (*runtime.ExecutionResult, error) {
	// First check this message satisfies all consensus rules before
	// applying the message. The rules include these clauses
	//
	// 1. the nonce of the message caller is correct
	// 2. caller has enough balance to cover transaction fee(gaslimit * gasprice)
	// 3. the amount of gas required is available in the block
	// 4. there is no overflow when calculating intrinsic gas
	// 5. the purchased gas is enough to cover intrinsic usage
	// 6. caller has enough balance to cover asset transfer for **topmost** call
	txn := t.State

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

	// 4. there is no overflow when calculating intrinsic gas
	intrinsicGasCost, err := TransactionGasCost(msg, t.Config.Homestead, t.Config.Istanbul)
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
	t.Ctx.GasPrice = types.BytesToHash(gasPrice.Bytes())
	t.Ctx.Origin = msg.From

	var result *runtime.ExecutionResult
	if msg.IsContractCreation() {
		result = t.Create2(msg.From, msg.Input, value, gasLeft)
	} else {
		txn.IncrNonce(msg.From)
		result = t.Call2(msg.From, *msg.To, msg.Input, value, gasLeft)
	}

	refund := txn.GetRefund()
	result.UpdateGasUsed(msg.Gas, refund)

	// refund the sender
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(result.GasLeft), gasPrice)
	txn.AddBalance(msg.From, remaining)

	// pay the coinbase
	coinbaseFee := new(big.Int).Mul(new(big.Int).SetUint64(result.GasUsed), gasPrice)
	txn.AddBalance(t.Ctx.Coinbase, coinbaseFee)

	// return gas to the pool
	t.addGasPool(result.GasLeft)

	return result, nil
}

func (t *Transition1) Create2(
	caller types.Address,
	code []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	address := crypto.CreateAddress(caller, t.State.GetNonce(caller))
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)

	return t.applyCreate(contract, t)
}

func (t *Transition1) Call2(
	caller types.Address,
	to types.Address,
	input []byte,
	value *big.Int,
	gas uint64,
) *runtime.ExecutionResult {
	c := runtime.NewContractCall(1, caller, caller, to, value, gas, t.State.GetCode(to), input)

	return t.applyCall(c, runtime.Call, t)
}

func (t *Transition1) run(contract *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {

	// check precompiled
	if t.precompiled.CanRun(contract, host, &t.Config) {
		return t.precompiled.Run(contract, host, &t.Config)
	}

	// check evm
	if t.evm.CanRun(contract, host, &t.Config) {
		return t.evm.Run(contract, host, &t.Config)
	}

	return &runtime.ExecutionResult{
		Err: fmt.Errorf("not found"),
	}
}

func (t *Transition1) transfer(from, to types.Address, amount *big.Int) error {
	if amount == nil {
		return nil
	}

	if err := t.State.SubBalance(from, amount); err != nil {
		if errors.Is(err, runtime.ErrNotEnoughFunds) {
			return runtime.ErrInsufficientBalance
		}

		return err
	}

	t.State.AddBalance(to, amount)

	return nil
}

func (t *Transition1) applyCall(
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

	snapshot := t.State.Snapshot()
	t.State.TouchAccount(c.Address)

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
		t.State.RevertToSnapshot(snapshot)
	}

	return result
}

var emptyHash types.Hash

func (t *Transition1) hasCodeOrNonce(addr types.Address) bool {
	nonce := t.State.GetNonce(addr)
	if nonce != 0 {
		return true
	}

	codeHash := t.State.GetCodeHash(addr)

	if codeHash != emptyCodeHashTwo && codeHash != emptyHash {
		return true
	}

	return false
}

func (t *Transition1) applyCreate(c *runtime.Contract, host runtime.Host) *runtime.ExecutionResult {
	gasLimit := c.Gas

	if c.Depth > int(1024)+1 {
		return &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     runtime.ErrDepth,
		}
	}

	// Increment the nonce of the caller
	t.State.IncrNonce(c.Caller)

	// Check if there if there is a collision and the address already exists
	if t.hasCodeOrNonce(c.Address) {
		return &runtime.ExecutionResult{
			GasLeft: 0,
			Err:     runtime.ErrContractAddressCollision,
		}
	}

	// Take snapshot of the current state
	snapshot := t.State.Snapshot()

	if t.Config.EIP158 {
		// Force the creation of the account
		t.State.CreateAccount(c.Address)
		t.State.IncrNonce(c.Address)
	}

	// Transfer the value
	if err := t.transfer(c.Caller, c.Address, c.Value); err != nil {
		return &runtime.ExecutionResult{
			GasLeft: gasLimit,
			Err:     err,
		}
	}

	result := t.run(c, host)

	if result.Failed() {
		t.State.RevertToSnapshot(snapshot)

		return result
	}

	if t.Config.EIP158 && len(result.ReturnValue) > spuriousDragonMaxCodeSize {
		// Contract size exceeds 'SpuriousDragon' size limit
		t.State.RevertToSnapshot(snapshot)

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
		if t.Config.Homestead {
			t.State.RevertToSnapshot(snapshot)

			result.GasLeft = 0
		}

		return result
	}

	result.GasLeft -= gasCost
	t.State.SetCode(c.Address, result.ReturnValue)

	return result
}

func (t *Transition1) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
	return t.State.SetStorage(addr, key, value, config)
}

func (t *Transition1) GetTxContext() runtime.TxContext {
	return t.Ctx
}

func (t *Transition1) GetBlockHash(number int64) (res types.Hash) {
	return t.GetHash(uint64(number))
}

func (t *Transition1) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	t.State.EmitLog(addr, topics, data)
}

func (t *Transition1) GetCodeSize(addr types.Address) int {
	return t.State.GetCodeSize(addr)
}

func (t *Transition1) GetCodeHash(addr types.Address) (res types.Hash) {
	return t.State.GetCodeHash(addr)
}

func (t *Transition1) GetCode(addr types.Address) []byte {
	return t.State.GetCode(addr)
}

func (t *Transition1) GetBalance(addr types.Address) *big.Int {
	return t.State.GetBalance(addr)
}

func (t *Transition1) GetStorage(addr types.Address, key types.Hash) types.Hash {
	return t.State.GetState(addr, key)
}

func (t *Transition1) AccountExists(addr types.Address) bool {
	return t.State.Exist(addr)
}

func (t *Transition1) Empty(addr types.Address) bool {
	return t.State.Empty(addr)
}

func (t *Transition1) GetNonce(addr types.Address) uint64 {
	return t.State.GetNonce(addr)
}

func (t *Transition1) Selfdestruct(addr types.Address, beneficiary types.Address) {
	if !t.State.HasSuicided(addr) {
		t.State.AddRefund(24000)
	}

	t.State.AddBalance(beneficiary, t.State.GetBalance(addr))
	t.State.Suicide(addr)
}

func (t *Transition1) Callx(c *runtime.Contract, h runtime.Host) *runtime.ExecutionResult {
	if c.Type == runtime.Create {
		return t.applyCreate(c, h)
	}

	return t.applyCall(c, c.Type, h)
}

// SetAccountDirectly sets an account to the given address
// NOTE: SetAccountDirectly changes the world state without a transaction
func (t *Transition1) SetAccountDirectly(addr types.Address, account *chain.GenesisAccount) error {
	if t.AccountExists(addr) {
		return fmt.Errorf("can't add account to %+v because an account exists already", addr)
	}

	t.State.SetCode(addr, account.Code)

	for key, value := range account.Storage {
		t.State.SetStorage(addr, key, value, &t.Config)
	}

	t.State.SetBalance(addr, account.Balance)
	t.State.SetNonce(addr, account.Nonce)

	return nil
}

// SetCodeDirectly sets new code into the account with the specified address
// NOTE: SetCodeDirectly changes the world state without a transaction
func (t *Transition1) SetCodeDirectly(addr types.Address, code []byte) error {
	if !t.AccountExists(addr) {
		return fmt.Errorf("account doesn't exist at %s", addr)
	}

	t.State.SetCode(addr, code)

	return nil
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
