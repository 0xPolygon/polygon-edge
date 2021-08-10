package system

import (
	"errors"
	"math/big"

	"github.com/0xPolygon/minimal/staking"
	"github.com/0xPolygon/minimal/types"
)

// unstakingHandler implements the unstaking logic for the System runtime
type unstakingHandler struct {
	s *System
}

// gas returns the fixed gas price of the unstaking operation
func (uh *unstakingHandler) gas(_ []byte) uint64 {
	return 40000
}

// run executes the system contract unstaking method
// Unstaking is for the FULL amount staked, no partial support is currently present
func (uh *unstakingHandler) run(state *systemState) ([]byte, error) {
	// Grab the staking address
	stakingAddress := types.StringToAddress(StakingAddress)

	// Grab the address calling the staking method
	staker := state.contract.Caller

	// Grab the last recorded staked balance for the account
	stakedBalance := state.host.GetStakedBalance(staker)

	// Grab the balance on the staking address
	stakingAccountBalance := state.host.GetBalance(stakingAddress)

	// Grab the transaction context
	ctx := state.host.GetTxContext()

	// Construct the pending event
	pendingEvent := staking.PendingEvent{
		BlockNumber: ctx.Number,
		Address:     staker,
		Value:       big.NewInt(0),
		EventType:   staking.UnstakingEvent,
	}

	// Find the staked balance after any possible events occur
	afterEventsStake := state.host.ComputeStakeAfterEvents(stakedBalance, pendingEvent)
	zeroValue := big.NewInt(0)

	// Sanity check
	// Can't unstake if the balance isn't previously present on the staking account
	// Can't unstake if the value is different from 0
	// Can't unstake if the account doesn't have stake
	// Can't unstake if the account has pending events which cause it to not have stake
	if stakingAccountBalance.Cmp(afterEventsStake) < 0 ||
		state.contract.Value.Cmp(zeroValue) != 0 ||
		afterEventsStake.Cmp(zeroValue) == 0 {
		return nil, errors.New("Invalid unstake request")
	}

	// Add a pending event to for the StakingHub
	staking.GetStakingHub().AddPendingEvent(pendingEvent)

	// Decrease the staked balance on the staking address
	state.host.SubBalance(stakingAddress, afterEventsStake)

	// Increase the account's actual balance
	state.host.AddBalance(staker, afterEventsStake)

	// Emit an unstaked event for the consensus layer
	state.host.EmitUnstakedEvent(staker, afterEventsStake)

	return nil, nil
}
