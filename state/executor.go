package state

import (
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/state/runtime/evm"
	"github.com/umbracle/minimal/state/runtime/precompiled"
)

const (
	CallCreateDepth uint64 = 1024
	MaxCodeSize            = 24576
)

var emptyCodeHashTwo = crypto.Keccak256Hash(nil)

var builtinRuntimes = map[string]runtime.Runtime{
	"evm": &evm.EVM{},
}

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) common.Hash

type CanTransferFunc func(*Txn, common.Address, *big.Int) bool

type TransferFunc func(state *Txn, from common.Address, to common.Address, amount *big.Int) error

type Executor struct {
	runtime runtime.Runtime

	config   chain.ForksInTime
	gasTable chain.GasTable

	state *Txn
	env   *runtime.Env

	getHash     GetHashByNumber
	CanTransfer CanTransferFunc
	Transfer    TransferFunc

	precompiled map[common.Address]*precompiled.Precompiled
}

func NewExecutor(state *Txn, env *runtime.Env, config chain.ForksInTime, gasTable chain.GasTable, getHash GetHashByNumber) *Executor {
	evm := evm.NewEVM(nil, state, env, config, gasTable, nil)

	return &Executor{
		runtime:     evm,
		config:      config,
		gasTable:    gasTable,
		state:       state,
		env:         env,
		getHash:     getHash,
		CanTransfer: CanTransfer,
		Transfer:    Transfer,
	}
}

// SetPrecompiled sets the precompiled contracts
func (e *Executor) SetPrecompiled(precompiled map[common.Address]*precompiled.Precompiled) {
	e.precompiled = precompiled
}

func (e *Executor) getPrecompiled(addr common.Address) (precompiled.Backend, bool) {
	p, ok := e.precompiled[addr]
	if !ok {
		return nil, false
	}
	if p.ActiveAt > e.env.Number.Uint64() {
		return nil, false
	}
	return p.Backend, true
}

func (e *Executor) Apply(txn *Txn, msg *types.Message, env *runtime.Env, gasTable chain.GasTable, config chain.ForksInTime, getHash evm.GetHashByNumber, gasPool GasPool, dryRun bool, builtins map[common.Address]*precompiled.Precompiled) (uint64, bool, error) {
	s := txn.Snapshot()
	gas, failed, err := e.apply(txn, msg, env, gasTable, config, getHash, gasPool, dryRun, builtins)
	if err != nil {
		txn.RevertToSnapshot(s)
	}
	return gas, failed, err
}

func (e *Executor) apply(txn *Txn, msg *types.Message, env *runtime.Env, gasTable chain.GasTable, config chain.ForksInTime, getHash evm.GetHashByNumber, gasPool GasPool, dryRun bool, builtins map[common.Address]*precompiled.Precompiled) (uint64, bool, error) {
	e.SetPrecompiled(builtins)

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
		// e := evm.NewEVM(txn, env, config, gasTable, getHash)
		// e.SetPrecompiled(builtins)

		// Weird
		e.runtime = evm.NewEVM(e, txn, env, config, gasTable, getHash)

		if contractCreation {
			_, txn.gas, vmerr = e.Create2(sender, msg.Data(), msg.Value(), txn.gas)
		} else {
			txn.SetNonce(msg.From(), txn.GetNonce(msg.From())+1)
			_, txn.gas, vmerr = e.Call2(sender, *msg.To(), msg.Data(), msg.Value(), txn.gas)
		}
		if vmerr != nil {
			if vmerr == runtime.ErrNotEnoughFunds {
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

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(txn.gas), msg.GasPrice())

	txn.AddBalance(msg.From(), remaining)

	// pay the coinbase
	txn.AddBalance(env.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(txn.gasUsed()), msg.GasPrice()))

	// Return remaining gas to the pool for the block
	gasPool.AddGas(txn.gas)

	return txn.gasUsed(), vmerr != nil, nil
}

func (e *Executor) Create2(caller common.Address, code []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	address := crypto.CreateAddress(caller, e.state.GetNonce(caller))
	contract := runtime.NewContractCreation(1, caller, caller, address, value, gas, code)
	return e.Create(contract)
}

func (e *Executor) Call2(caller common.Address, to common.Address, input []byte, value *big.Int, gas uint64) ([]byte, uint64, error) {
	c := runtime.NewContractCall(1, caller, caller, to, value, gas, e.state.GetCode(to), input)
	return e.Call(c, runtime.Call)
}

func (e *Executor) Call(c *runtime.Contract, t runtime.CallType) ([]byte, uint64, error) {
	// Check if its too deep
	if c.Depth() > int(CallCreateDepth)+1 {
		return nil, c.Gas, runtime.ErrDepth
	}

	// Check if there is enough balance
	if t == runtime.Call || t == runtime.CallCode {
		if !e.CanTransfer(e.state, c.Caller, c.Value) {
			return nil, c.Gas, runtime.ErrNotEnoughFunds
		}
	}

	snapshot := e.state.Snapshot()

	if t == runtime.Call {
		_, isPrecompiled := e.getPrecompiled(c.CodeAddress)
		if !e.state.Exist(c.Address) {
			if !isPrecompiled && e.config.EIP158 && c.Value.Sign() == 0 {
				// calling an unexisting account
				return nil, c.Gas, nil
			}

			// Not sure why but the address has to be created for the precompiled contracts
			e.state.CreateAccount(c.Address)
		}

		// Try to transfer
		if err := e.Transfer(e.state, c.Caller, c.Address, c.Value); err != nil {
			return nil, c.Gas, err // FIX
		}
	}

	ret, gas, err := e.run(c)

	if err != nil {
		e.state.RevertToSnapshot(snapshot)
		if err != runtime.ErrExecutionReverted {
			gas = 0
		}
	}

	return ret, gas, err
}

func (e *Executor) run(contract *runtime.Contract) ([]byte, uint64, error) {

	// check precompiled first
	precompiled, isPrecompiled := e.getPrecompiled(contract.CodeAddress)
	if isPrecompiled {
		if !contract.ConsumeGas(precompiled.Gas(contract.Input)) {
			// revert to snapshot?
			return nil, 0, runtime.ErrGasOverflow
		}

		ret, err := precompiled.Call(contract.Input)
		return ret, contract.Gas, err
	}

	return e.runtime.Run(contract)
}

func (e *Executor) Create(contract *runtime.Contract) ([]byte, uint64, error) {
	gas := contract.Gas

	// Check if its too deep
	if contract.Depth() > int(CallCreateDepth)+1 {
		return nil, gas, runtime.ErrDepth
	}

	caller, address, value := contract.Caller, contract.Address, contract.Value

	// Check if the values can be transfered
	if !e.CanTransfer(e.state, caller, value) {
		return nil, gas, runtime.ErrNotEnoughFunds
	}

	// Increase the nonce of the caller
	nonce := e.state.GetNonce(caller)
	e.state.SetNonce(caller, nonce+1)

	// Check for address collisions
	contractHash := e.state.GetCodeHash(address)
	if e.state.GetNonce(address) != 0 || (contractHash != (common.Hash{}) && contractHash != emptyCodeHashTwo) {
		return nil, 0, runtime.ErrContractAddressCollision
	}

	// Take snapshot of the current state
	snapshot := e.state.Snapshot()

	// Create the new account for the contract
	e.state.CreateAccount(address)
	if e.config.EIP158 {
		e.state.SetNonce(address, 1)
	}

	// Transfer the value
	if value != nil {
		if err := e.Transfer(e.state, caller, address, value); err != nil {
			return nil, gas, runtime.ErrNotEnoughFunds
		}
	}

	// run the code here
	ret, gas, err := e.run(contract)
	contract.Gas = gas // REMOVE: THIS IS ONLY DONE TO REUSE THE CONSUME GAS FUNCTIONS

	maxCodeSizeExceeded := e.config.EIP158 && len(ret) > MaxCodeSize

	if err == nil && !maxCodeSizeExceeded {
		createDataGas := uint64(len(ret)) * params.CreateDataGas
		if contract.ConsumeGas(createDataGas) {
			e.state.SetCode(address, ret)
		} else {
			err = vm.ErrCodeStoreOutOfGas
		}
	}

	if maxCodeSizeExceeded || (err != nil && (e.config.Homestead || err != vm.ErrCodeStoreOutOfGas)) {
		e.state.RevertToSnapshot(snapshot)
		if err != runtime.ErrExecutionReverted {
			contract.ConsumeAllGas()
		}
	}

	// Assign err if contract code size exceeds the max while the err is still empty.
	if maxCodeSizeExceeded && err == nil {
		err = runtime.ErrMaxCodeSizeExceeded
	}

	return ret, contract.Gas, err
}

func CanTransfer(state *Txn, from common.Address, amount *big.Int) bool {
	return state.GetBalance(from).Cmp(amount) >= 0
}

func Transfer(state *Txn, from common.Address, to common.Address, amount *big.Int) error {
	if balance := state.GetBalance(from); balance.Cmp(amount) < 0 {
		return runtime.ErrNotEnoughFunds
	}

	state.SubBalance(from, amount)
	state.AddBalance(to, amount)
	return nil
}
