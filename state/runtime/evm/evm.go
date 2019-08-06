package evm

import (
	"math/big"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
	"github.com/umbracle/minimal/types"
)

// GetHashByNumber returns the hash function of a block number
type GetHashByNumber = func(i uint64) types.Hash

type CanTransferFunc func(runtime.State, types.Address, *big.Int) bool

type TransferFunc func(state runtime.State, from types.Address, to types.Address, amount *big.Int) error

// EVM is the ethereum virtual machine
type EVM struct {
	config   chain.ForksInTime
	gasTable chain.GasTable

	state runtime.State
	env   *runtime.Env

	getHash     GetHashByNumber
	CanTransfer CanTransferFunc
	Transfer    TransferFunc

	executor runtime.Executor
}

func (e *EVM) Run(c *runtime.Contract) ([]byte, uint64, error) {
	contract := &state{
		ip:          0,
		depth:       c.Depth(),
		code:        c.Code,
		evm:         e,
		caller:      c.Caller,
		origin:      c.Origin,
		codeAddress: c.CodeAddress,
		address:     c.Address,
		value:       c.Value,
		stack:       []*big.Int{},
		sp:          0,
		gas:         c.Gas,
		input:       c.Input,
		bitvec:      codeBitmap(c.Code),
		static:      c.Static,
		memory:      []byte{},
	}

	ret, err := contract.Run()
	return ret, contract.gas, err
}

// NewEVM creates a new EVM
func NewEVM(executor runtime.Executor, state runtime.State, env *runtime.Env, config chain.ForksInTime, gasTable chain.GasTable, getHash GetHashByNumber) *EVM {
	return &EVM{
		config:   config,
		gasTable: gasTable,
		executor: executor,
		state:    state,
		env:      env,
		getHash:  getHash,
	}
}
