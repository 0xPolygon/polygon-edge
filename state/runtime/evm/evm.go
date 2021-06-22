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
func (e *EVM) CanRun(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) bool {
	return true
}

// Name implements the runtime interface
func (e *EVM) Name() string {
	return "evm"
}

// Run implements the runtime interface
func (e *EVM) Run(c *runtime.Contract, host runtime.Host, config *chain.ForksInTime) ([]byte, uint64, error) {

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

	rett := []byte{}
	rett = append(rett[:0], ret...)

	gas := contract.gas

	releaseState(contract)

	if err != nil && err != runtime.ErrExecutionReverted {
		gas = 0
	}

	return rett, gas, err
}
