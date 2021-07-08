package system

// stakingHandler implements the staking logic for the System runtime
type stakingHandler struct {
	s *System
}

// gas returns the fixed gas price of the staking operation
func (sh *stakingHandler) gas(_ []byte) uint64 {
	return 40000
}

// run executes the system contract staking method
func (sh *stakingHandler) run(state *systemState) ([]byte, error) {
	// Grab the value being staked
	potentialStake := state.contract.Value

	// Grab the address calling the staking method
	staker := state.contract.Caller

	// Increase the account's staked balance
	state.host.AddStakedBalance(staker, potentialStake)

	state.host.EmitStakedEvent(staker, potentialStake)

	return nil, nil
}
