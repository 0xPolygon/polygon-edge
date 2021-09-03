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

	validators, err := i.getNextValidators(header)
	if err != nil {
		return err
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
		newSnap.Number = header.Number
		newSnap.Hash = header.Hash.String()

		if snap.Number != header.Number {
			i.store.add(newSnap)
		} else {
			i.store.replace(newSnap)
		}
	}
	return nil
}

func (i *Ibft) batchUpdateValidators(from, to uint64) error {
	for n := from; n <= to; n++ {
		if i.IsLastOfEpoch(n) {
			if err := i.updateValidators(n); err != nil {
				return err
			}
		}
	}
	return nil
}

func (i *Ibft) GetEpoch(number uint64) uint64 {
	if number%i.epochSize == 0 {
		return number / i.epochSize
	} else {
		return number/i.epochSize + 1
	}
}

func (i *Ibft) IsFirstOfEpoch(number uint64) bool {
	return number%i.epochSize == 1
}

func (i *Ibft) IsLastOfEpoch(number uint64) bool {
	return number > 0 && number%i.epochSize == 0
}
