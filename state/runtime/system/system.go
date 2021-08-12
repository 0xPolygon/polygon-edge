package system

import (
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/state/runtime"
	"github.com/0xPolygon/minimal/types"
)

var _ runtime.Runtime = &System{}

// System is the implementation for the system (staking) runtime
type System struct {
	systemContracts map[types.Address]systemContract
}

// systemContract defines the interface each system contract needs to implement
type systemContract interface {
	// gas defines how much gas the system contract takes up (fixed)
	gas(input []byte) uint64

	// run defines the system contract implementation
	run(state *systemState) ([]byte, error)
}

var (
	StakingAddress   = "1001"
	UnstakingAddress = "1002"
)

// setupHandlers defines which addresses are assigned to which system contract handlers
func (s *System) setupHandlers() {
	s.registerHandler(StakingAddress, &stakingHandler{s})
	s.registerHandler(UnstakingAddress, &unstakingHandler{s})
}

// registerHandler registers a new systemContract handler for the specified address
func (s *System) registerHandler(addr string, sysContract systemContract) {
	// Create the map if it doesn't exist
	if len(s.systemContracts) == 0 {
		s.systemContracts = map[types.Address]systemContract{}
	}

	// Register the handler to the map
	// [MAP] address -> handler
	s.systemContracts[types.StringToAddress(addr)] = sysContract
}

// NewSystem creates a runtime for the system contracts
func NewSystem() *System {
	s := &System{}
	s.setupHandlers()

	return s
}

// Run represents the actual runtime implementation, after the CanRun check passes
func (s *System) Run(contract *runtime.Contract, host runtime.Host, _ *chain.ForksInTime) *runtime.ExecutionResult {
	// Get the system state from the pool and set it up
	sysState := acquireSystemState()
	sysState.host = host
	sysState.contract = contract

	// Grab the corresponding system contract
	sysContract := s.systemContracts[contract.CodeAddress]

	// Calculate the gas cost, see if there is an overflow
	gasCost := sysContract.gas(contract.Input)
	if contract.Gas < gasCost {
		return &runtime.ExecutionResult{
			ReturnValue: nil,
			GasLeft:     0,
			Err:         runtime.ErrGasOverflow,
		}
	}
	contract.Gas = contract.Gas - gasCost

	// Run the system contract
	ret, err := sysContract.run(sysState)

	// Put the system state back into the pool
	releaseSystemState(sysState)

	result := &runtime.ExecutionResult{
		ReturnValue: ret,
		GasLeft:     contract.Gas,
		Err:         err,
	}

	return result
}

// CanRun checks if the current runtime can execute the query
func (s *System) CanRun(c *runtime.Contract, _ runtime.Host, _ *chain.ForksInTime) bool {
	// Check if the system contract address is registered to a handler
	if _, canRun := s.systemContracts[c.CodeAddress]; !canRun {
		return false
	}

	return true
}

// Name returns the name of the runtime
func (s *System) Name() string {
	return "system"
}
