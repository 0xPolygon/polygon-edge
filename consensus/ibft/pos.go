package ibft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/minimal/state"
	"github.com/0xPolygon/minimal/types"
)

// SubscribeStakingEvent subscribes staking events that matches conditions and returns stop subscription function
// stop subscription function stops subscription and returns received events
func (i *Ibft) SubscribeStakingEvent(f func(e *state.StakingEvent) bool) func() []*state.StakingEvent {
	resCh := make(chan []*state.StakingEvent, 1)
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

	// this function stops subscription and return received events
	return func() []*state.StakingEvent {
		i.executor.UnsubscribeStakingEvent(subscription)
		subscription.WaitForDone()
		close(subscription.EventCh)
		return <-resCh
	}
}

// getNextValidatorSet returns the validator set for the next
func (i *Ibft) getNextValidatorSet(header *types.Header, stakingEvents []*state.StakingEvent) (ValidatorSet, error) {
	transition, err := i.executor.BeginTxn(header.StateRoot, header, header.Miner)
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

	threshold := big.NewInt(0)
	nextValidators := make(ValidatorSet, 0, len(snap.Set))
	isChecked := make(map[types.Address]bool)

	// Check staked balance of current validators
	for _, v := range snap.Set {
		isChecked[v] = true
		stakedBalance := transition.GetStakedBalance(v)
		if stakedBalance.Cmp(threshold) == 1 {
			nextValidators.Add(v)
		}
	}
	// Check staker/unstaker
	for _, e := range stakingEvents {
		if !isChecked[e.Address] {
			isChecked[e.Address] = true

			stakedBalance := transition.GetStakedBalance(e.Address)
			if stakedBalance.Cmp(threshold) == 1 {
				nextValidators.Add(e.Address)
			}
		}
	}

	return nextValidators, nil
}

// updateSnapshotValidators update validators in snapshot at given height
func (i *Ibft) updateSnapshotValidators(num uint64, validators ValidatorSet) error {
	snap, err := i.getSnapshot(num)
	if err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("cannot find snapshot at %d", num)
	}
	if !snap.Set.Equal(&validators) {
		newSnap := snap.Copy()
		newSnap.Set = validators
		if snap.Number != num {
			// create new one
			newSnap.Number = num
			newSnap.Hash = ""
			i.store.add(newSnap)
		} else {
			i.store.replace(newSnap)
		}
	}
	return nil
}

// bulkUpdateSnapshots updates validators in multiple snapshots
func (i *Ibft) bulkUpdateSnapshots(begin, end uint64, events []*state.StakingEvent) error {
	for n := begin; n <= end; n++ {
		header, ok := i.blockchain.GetHeaderByNumber(n)
		if !ok {
			return fmt.Errorf("cannot find header at %d", n)
		}
		// get events happened at given height
		es := []*state.StakingEvent{}
		for _, e := range events {
			if uint64(e.Number) == n {
				es = append(es, e)
			}
		}
		validators, err := i.getNextValidatorSet(header, es)
		if err != nil {
			return err
		}
		i.updateSnapshotValidators(n, validators)
	}
	return nil
}
