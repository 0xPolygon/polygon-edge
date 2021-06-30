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

// updateSnapshotValidators overwrite validators in snapshot at given height
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
		newSnap.Number = num
		newSnap.Hash = ""
		newSnap.Set = validators
		i.store.add(newSnap)
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

		snap, err := i.getSnapshot(n)
		if err != nil {
			return err
		}
		if snap == nil {
			return fmt.Errorf("cannot find snapshot at %d", n)
		}
		if !snap.Set.Equal(&validators) {
			newSnap := snap.Copy()
			newSnap.Number = header.Number
			newSnap.Hash = header.Hash.String()
			newSnap.Set = validators

			if snap.Number == newSnap.Number {
				i.store.replace(newSnap)
			} else {
				i.store.add(newSnap)
			}
		}
	}
	return nil
}
