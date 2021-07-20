package evm

import (
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state/runtime"
)

var _ runtime.Runtime = &EVM{}

// EVM is the ethereum virtual machine
type EVM struct {
}

// NewEVM creates a new EVM
func NewEVM() *EVM {
	return &EVM{}
}

// CanRun implements the runtime interface
func (e *EVM) CanRun(_ *runtime.Contract, _ runtime.Host, _ *chain.ForksInTime) bool {
	return true
}

// Name implements the runtime interface
func (e *EVM) Name() string {
	return "evm"
}

// Run implements the runtime interface
func (e *EVM) Run(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) (returnValue []byte, gasLeft uint64, err error) {

	contract := acquireState()
	contract.resetReturnData()

	contract.msg = c
	contract.code = c.Code
	contract.evm = e
	contract.gas = c.Gas
	contract.host = host
	contract.config = config

	contract.bitmap.setCode(c.Code)

	ret, err := contract.Run()

	// We are probably doing this append magic to make sure that the slice doesn't have more capacity than it needs
	returnValue = append(returnValue[:0], ret...)

	gasLeft = contract.gas

	releaseState(contract)

	if err != nil && err != runtime.ErrExecutionReverted {
		gasLeft = 0
	}

	return returnValue, gasLeft, err
}
