package ibft

import (
	"fmt"

	"github.com/0xPolygon/minimal/state/runtime/system"
	"github.com/0xPolygon/minimal/types"
)

// GetSystemContractValidators returns validator set in system contract at given header
func (i *Ibft) GetSystemContractValidators(header *types.Header) (ValidatorSet, error) {
	transition, err := i.executor.BeginTxn(header.StateRoot, header)
	if err != nil {
		return nil, err
	}
	return system.GetValidators(transition), err
}

// GetNextValidators merges two validator set in snapshot and contract
func (i *Ibft) GetNextValidators(header *types.Header) (ValidatorSet, error) {
	snap, err := i.getSnapshot(header.Number)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("cannot find snapshot, num=%d", header.Number)
	}
	systemValidators, err := i.GetSystemContractValidators(header)
	if err != nil {
		return nil, err
	}

	nextValidators := ValidatorSet{}
	nextValidators = append(nextValidators, snap.Set...)
	for _, sv := range systemValidators {
		if !nextValidators.Includes(sv) {
			nextValidators.Add(sv)
		}
	}

	return nextValidators, nil
}
