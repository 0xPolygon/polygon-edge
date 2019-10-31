package evm

import (
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/state/runtime"
)

var _ runtime.Runtime = &EVM{}

// EVM is the ethereum virtual machine
type EVM struct {
	vs []state
}

// NewEVM creates a new EVM
func NewEVM() *EVM {
	return &EVM{}
}

/*
// TODO
func (c *EVM) getValue() *state {
	if cap(c.vs) > len(c.vs) {
		c.vs = c.vs[:len(c.vs)+1]
	} else {
		c.vs = append(c.vs, state{})
	}
	return &c.vs[len(c.vs)-1]
}
*/

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
