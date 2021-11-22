package ibft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	"github.com/0xPolygon/polygon-sdk/types"
)

// PoSMechanism defines specific hooks for the Proof of Stake IBFT mechanism
type PoSMechanism struct {
	// Reference to the main IBFT implementation
	ibft *Ibft

	// hookMap is the collection of registered hooks
	hookMap map[string]func(interface{}) error

	// Used for easy lookups
	mechanismType Type
}

// PoSFactory initializes the required data
// for the Proof of Stake mechanism
func PoSFactory() (ConsensusMechanism, error) {
	pos := &PoSMechanism{
		mechanismType: PoS,
	}

	// Initialize the hook map
	pos.initializeHookMap()

	return pos, nil
}

// GetType implements the ConsensusMechanism interface method
func (pos *PoSMechanism) GetType() Type {
	return pos.mechanismType
}

// GetHookMap implements the ConsensusMechanism interface method
func (pos *PoSMechanism) GetHookMap() map[string]func(interface{}) error {
	return pos.hookMap
}

// acceptStateLogHook logs the current snapshot
func (pos *PoSMechanism) acceptStateLogHook(snapParam interface{}) error {
	// Cast the param to a *Snapshot
	snap := snapParam.(*Snapshot)

	// Log the info message
	pos.ibft.logger.Info(
		"current snapshot",
		"validators",
		len(snap.Set),
	)

	return nil
}

// insertBlockHook checks if the block is the last block of the epoch,
// in order to update the validator set
func (pos *PoSMechanism) insertBlockHook(numberParam interface{}) error {
	headerNumber := numberParam.(uint64)

	if pos.ibft.IsLastOfEpoch(headerNumber) {
		if err := pos.ibft.updateValidators(headerNumber); err != nil {
			return err
		}
	}

	return nil
}

// initializeHookMap registers the hooks that the PoS mechanism
// should have
func (pos *PoSMechanism) initializeHookMap() {
	// Create the hook map
	pos.hookMap = make(map[string]func(interface{}) error)

	// Register the AcceptStateLogHook
	pos.hookMap[AcceptStateLogHook] = pos.acceptStateLogHook

	// Register the InsertBlockHook
	pos.hookMap[InsertBlockHook] = pos.insertBlockHook
}

// syncStateHookParams are the params used for the syncStateHook
type syncStateHookParams struct {
	oldLatestNumber uint64
	headerNumber    uint64
}

// getNextValidators is a helper function for fetching the validator set
// from the Staking SC
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

// batchUpdateValidators updates the validator set based on the passed in block range
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

// GetEpoch returns the current epoch
func (i *Ibft) GetEpoch(number uint64) uint64 {
	if number%i.epochSize == 0 {
		return number / i.epochSize
	}

	return number/i.epochSize + 1
}

// IsFirstOfEpoch checks if the block number is the first of the epoch
func (i *Ibft) IsFirstOfEpoch(number uint64) bool {
	return number%i.epochSize == 1
}

// IsLastOfEpoch checks if the block number is the last of the epoch
func (i *Ibft) IsLastOfEpoch(number uint64) bool {
	return number > 0 && number%i.epochSize == 0
}
