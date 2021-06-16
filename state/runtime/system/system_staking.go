package system

import (
	"errors"
	"fmt"
)

// stakingHandler implements the staking logic for the System runtime
type stakingHandler struct {
	s *System
}

// gas returns the fixed gas price of the staking operation
func (sh *stakingHandler) gas(_ []byte) uint64 {
	fmt.Print("\n\n[Staking Handler] Gas calculation called\n\n")

	return 40000
}

// run executes the system contract staking method
func (sh *stakingHandler) run(state *systemState) ([]byte, error) {
	fmt.Printf("\n\n[Staking Handler RUN]\n\n [STAKER]: %s\n[AMOUNT]: %s\n\n", state.contract.Caller, state.contract.Value)

	// Grab the value being staked
	potentialStake := state.contract.Value

	// Grab the address calling the staking method
	staker := state.contract.Caller

	// TODO Not sure if this check is necessary
	accountBalance := state.host.GetBalance(staker)
	if potentialStake.Cmp(accountBalance) > 0 {
		// The staker wants to stake a balance they can't afford
		return nil, errors.New("potential stake exceeds account balance")
	}

	// Reduce the staked amount from the account balance, 
	// increase the staked balance
	state.host.SubBalance(staker, potentialStake)
	state.host.AddStakedBalance(staker, potentialStake)

	// TODO Add the staker to the validator set after this point + checks

	return nil, nil
}
