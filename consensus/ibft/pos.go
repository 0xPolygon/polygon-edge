package ibft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// getValidatorSet returns validator set from parent snapshot and validator set in state
func (i *Ibft) getValidatorSet(parentSnap *Snapshot) ValidatorSet {
	// prioritize i.state.nextValidators
	validators := i.state.nextValidators
	if len(validators) == 0 {
		validators = parentSnap.Set
	}
	return validators
}

// getNextValidatorSet returns the validator set in the next sequence of given header
func (i *Ibft) getNextValidatorSet(header *types.Header, stakingEvents []*state.StakingEvent) (ValidatorSet, error) {
	transition, err := i.executor.BeginTxn(header.StateRoot, header)
	if err != nil {
		return nil, err
	}

	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("cannot find the snapshot at %d", header.Number)
	}

	nextValidators := make(ValidatorSet, 0, len(snap.Set))

	// FIXME: keep current validators for now because genesis validators haven't staked
	nextValidators = append(nextValidators, snap.Set...)

	threshold := big.NewInt(0)
	for _, e := range stakingEvents {
		if !nextValidators.Includes(e.Address) {
			stakedBalance := transition.GetStakedBalance(e.Address)
			if stakedBalance.Cmp(threshold) == 1 {
				nextValidators.Add(e.Address)
			}
		}
	}

	return nextValidators, nil
}

type void struct{}

// SubscribeStakingEvent returns 2 channels to get list of StakingEvent and notify to finish subscription
// list will be sent to resCh after send a signal to closeCh
func (i *Ibft) SubscribeStakingEvent(f func(e *state.StakingEvent) bool) (<-chan []*state.StakingEvent, chan<- void) {
	resCh := make(chan []*state.StakingEvent, 1)
	closeCh := make(chan void, 1)

	subscription := i.executor.SubscribeStakingEvent()
	go func() {
		events := []*state.StakingEvent{}
		// collect until channel ends
		for e := range subscription.EventCh {
			if f(e) {
				events = append(events, e)
			}
		}
		resCh <- events
	}()
	go func() {
		<-closeCh
		i.executor.UnsubscribeStakingEvent(subscription)
		// wait until all event reaches
		subscription.WaitForDone()
		close(subscription.EventCh)
		close(closeCh)
	}()

	return resCh, closeCh
}

// verifyValidatorSet verifies validator set in extra data of header equals to the validators in state
func (i *Ibft) verifyValidatorSet(header *types.Header) error {
	extra, err := getIbftExtra(header)
	if err != nil {
		return err
	}

	// check use same validator set
	extraValidatorSet := ValidatorSet(extra.Validators)
	stateValidatorSet := ValidatorSet(i.state.validators)
	if !extraValidatorSet.Equal(&stateValidatorSet) {
		return fmt.Errorf("wrong validator set")
	}
	return nil
}
