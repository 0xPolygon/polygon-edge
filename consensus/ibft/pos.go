package ibft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	"github.com/0xPolygon/polygon-sdk/types"
)

func (i *Ibft) getNextValidators(header *types.Header) (ValidatorSet, error) {
	transition, err := i.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
	if err != nil {
		return nil, err
	}
	return staking.QueryValidators(transition, i.validatorKeyAddr)
}

// updateSnapshotValidators updates validators in snapshot at given height
func (i *Ibft) updateValidators(num uint64) error {
	header, ok := i.blockchain.GetHeaderByNumber(num)
	if !ok {
		return errors.New("header not found")
	}

	validators, _ := i.getNextValidators(header)
	// ignore for now if it returns error or nothing
	if len(validators) == 0 {
		return nil
	}

	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		return err
	}
	if snap == nil {
		return fmt.Errorf("cannot find snapshot at %d", header.Number)
	}

	if !snap.Set.Equal(&validators) {
		newSnap := snap.Copy()
		newSnap.Set = validators
		if snap.Number != header.Number {
			newSnap.Number = header.Number
			newSnap.Hash = header.Hash.String()
			i.store.add(newSnap)
		} else {
			i.store.replace(newSnap)
		}
	}
	return nil
}

func (i *Ibft) batchUpdateValidators(from, to uint64) error {
	for n := from; n <= to; n++ {
		if err := i.updateValidators(n); err != nil {
			return err
		}
	}
	return nil
}
