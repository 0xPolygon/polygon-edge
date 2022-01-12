package ibft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/state"

	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-sdk/helper/staking"
	"github.com/0xPolygon/polygon-sdk/types"
)

// PoSMechanism defines specific hooks for the Proof of Stake IBFT mechanism
type PoSMechanism struct {
	BaseConsensusMechanism
	// Params
	ContractDeployment uint64 // The height when deploying staking contract
}

// PoSFactory initializes the required data
// for the Proof of Stake mechanism
func PoSFactory(ibft *Ibft, params map[string]interface{}) (ConsensusMechanism, error) {
	pos := &PoSMechanism{
		BaseConsensusMechanism: BaseConsensusMechanism{
			mechanismType: PoS,
			ibft:          ibft,
		},
	}

	if err := pos.initializeParams(params); err != nil {
		return nil, err
	}
	pos.initializeHookMap()

	return pos, nil
}

// initializeParams initializes mechanism parameters from chain config
func (pos *PoSMechanism) initializeParams(params map[string]interface{}) error {
	if len(params) == 0 {
		return nil
	}
	if err := pos.BaseConsensusMechanism.initializeParams(params); err != nil {
		return err
	}

	rawDeployment, ok := params["deployment"]
	if pos.From > 0 && !ok {
		return fmt.Errorf("deployment must be given if PoS starts in the middle")
	}
	if ok {
		deployment, err := common.ConvertUnmarshalledInt(rawDeployment)
		if err != nil {
			return fmt.Errorf(`failed to parse "deployment" params: %w`, err)
		}
		if deployment < 0 {
			return fmt.Errorf(`"deployment" must be zero or positive integer: %d`, deployment)
		}
		uDeployment := uint64(deployment)
		if uDeployment > pos.From {
			return fmt.Errorf(`"deployment" must be less than or equal to "from": deployment=%d, from=%d`, deployment, pos.From)
		}
		pos.ContractDeployment = uDeployment
	}

	return nil
}

// GetType implements the ConsensusMechanism interface method
func (pos *PoSMechanism) GetType() MechanismType {
	return pos.mechanismType
}

// GetHookMap implements the ConsensusMechanism interface method
func (pos *PoSMechanism) GetHookMap() map[string]func(interface{}) error {
	return pos.hookMap
}

// acceptStateLogHook logs the current snapshot
func (pos *PoSMechanism) acceptStateLogHook(snapParam interface{}) error {
	// Cast the param to a *Snapshot
	snap, ok := snapParam.(*Snapshot)
	if !ok {
		return ErrInvalidHookParam
	}

	if !pos.IsAvailable() {
		return nil
	}

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
	headerNumber, ok := numberParam.(uint64)
	if !ok {
		return ErrInvalidHookParam
	}

	if headerNumber+1 == pos.From || pos.IsAvailableAtNumber(headerNumber) && pos.ibft.IsLastOfEpoch(headerNumber) {
		return pos.updateValidators(headerNumber)
	}

	return nil
}

// verifyBlockHook checks if the block is an epoch block and if it has any transactions
func (pos *PoSMechanism) verifyBlockHook(blockParam interface{}) error {
	block, ok := blockParam.(*types.Block)
	if !ok {
		return ErrInvalidHookParam
	}

	blockNumber := block.Number()
	if !pos.IsAvailableAtNumber(blockNumber) {
		return nil
	}

	if pos.ibft.IsLastOfEpoch(blockNumber) && len(block.Transactions) > 0 {
		return errBlockVerificationFailed
	}

	return nil
}

// preStateCommitHookParams are the params passed into the preStateCommitHook
type preStateCommitHookParams struct {
	header *types.Header
	txn    *state.Transition
}

// TODO: check preStateCommitHook is called during bulk syncing
// verifyBlockHook checks if the block is an epoch block and if it has any transactions
func (pos *PoSMechanism) preStateCommitHook(rawParams interface{}) error {
	params, ok := rawParams.(*preStateCommitHookParams)
	if !ok {
		return ErrInvalidHookParam
	}
	if params.header.Number != pos.ContractDeployment {
		return nil
	}

	// Deploy Staking contract
	contractState, err := stakingHelper.PredeployStakingSC(nil)
	if err != nil {
		return err
	}
	if err := params.txn.ForceToDeployContract(staking.AddrStakingContract, contractState); err != nil {
		return err
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

	// Register the VerifyBlockHook
	pos.hookMap[VerifyBlockHook] = pos.verifyBlockHook

	// Register the PreStateCommitHook
	pos.hookMap[PreStateCommitHook] = pos.preStateCommitHook
}

// ShouldWriteTransactions indicates if transactions should be written to a block
func (pos *PoSMechanism) ShouldWriteTransactions(blockNumber uint64) bool {
	// Epoch blocks should be empty
	return pos.IsAvailableAtNumber(blockNumber) && !pos.ibft.IsLastOfEpoch(blockNumber)
}

// getNextValidators is a helper function for fetching the validator set
// from the Staking SC
func (pos *PoSMechanism) getNextValidators(header *types.Header) (ValidatorSet, error) {
	transition, err := pos.ibft.executor.BeginTxn(header.StateRoot, header, types.ZeroAddress)
	if err != nil {
		return nil, err
	}
	return staking.QueryValidators(transition, pos.ibft.validatorKeyAddr)
}

// updateSnapshotValidators updates validators in snapshot at given height
func (pos *PoSMechanism) updateValidators(num uint64) error {
	header, ok := pos.ibft.blockchain.GetHeaderByNumber(num)
	if !ok {
		return errors.New("header not found")
	}

	validators, err := pos.getNextValidators(header)
	if err != nil {
		return err
	}

	snap, err := pos.ibft.getSnapshot(header.Number)
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
			pos.ibft.store.add(newSnap)
		} else {
			pos.ibft.store.replace(newSnap)
		}
	}
	return nil
}
