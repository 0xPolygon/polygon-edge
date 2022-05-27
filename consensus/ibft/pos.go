package ibft

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

// PoSMechanism defines specific hooks for the Proof of Stake IBFT mechanism
type PoSMechanism struct {
	BaseConsensusMechanism
	// Params
	ContractDeployment uint64 // The height when deploying staking contract
	MaxValidatorCount  uint64
	MinValidatorCount  uint64
}

// PoSFactory initializes the required data
// for the Proof of Stake mechanism
func PoSFactory(ibft *Ibft, params *IBFTFork) (ConsensusMechanism, error) {
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

// IsAvailable returns indicates if mechanism should be called at given height
func (pos *PoSMechanism) IsAvailable(hookType HookType, height uint64) bool {
	switch hookType {
	case AcceptStateLogHook, VerifyBlockHook, CalculateProposerHook:
		return pos.IsInRange(height)
	case PreStateCommitHook:
		// deploy contract on ContractDeployment
		return height == pos.ContractDeployment
	case InsertBlockHook:
		// update validators when the one before the beginning or the end of epoch
		return height+1 == pos.From || pos.IsInRange(height) && pos.ibft.IsLastOfEpoch(height)
	default:
		return false
	}
}

// initializeParams initializes mechanism parameters from chain config
func (pos *PoSMechanism) initializeParams(params *IBFTFork) error {
	if err := pos.BaseConsensusMechanism.initializeParams(params); err != nil {
		return err
	}

	if pos.From != 0 {
		if params.Deployment == nil {
			return errors.New(`"deployment" must be specified in PoS fork`)
		}

		if params.Deployment.Value > pos.From {
			return fmt.Errorf(
				`"deployment" must be less than or equal to "from": deployment=%d, from=%d`,
				params.Deployment.Value,
				pos.From,
			)
		}

		pos.ContractDeployment = params.Deployment.Value

		if params.MaxValidatorCount == nil {
			pos.MaxValidatorCount = stakingHelper.MaxValidatorCount
		} else {
			pos.MaxValidatorCount = params.MaxValidatorCount.Value
		}

		if params.MinValidatorCount == nil {
			pos.MinValidatorCount = stakingHelper.MinValidatorCount
		} else {
			pos.MinValidatorCount = params.MinValidatorCount.Value
		}
	}

	return nil
}

// calculateProposerHook calculates the next proposer based on the last
func (pos *PoSMechanism) calculateProposerHook(lastProposerParam interface{}) error {
	lastProposer, ok := lastProposerParam.(types.Address)
	if !ok {
		return ErrInvalidHookParam
	}

	pos.ibft.state.CalcProposer(lastProposer)

	return nil
}

// acceptStateLogHook logs the current snapshot
func (pos *PoSMechanism) acceptStateLogHook(snapParam interface{}) error {
	// Cast the param to a *Snapshot
	snap, ok := snapParam.(*Snapshot)
	if !ok {
		return ErrInvalidHookParam
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

	return pos.updateValidators(headerNumber)
}

// verifyBlockHook checks if the block is an epoch block and if it has any transactions
func (pos *PoSMechanism) verifyBlockHook(blockParam interface{}) error {
	block, ok := blockParam.(*types.Block)
	if !ok {
		return ErrInvalidHookParam
	}

	if pos.ibft.IsLastOfEpoch(block.Number()) && len(block.Transactions) > 0 {
		return errBlockVerificationFailed
	}

	return nil
}

// preStateCommitHookParams are the params passed into the preStateCommitHook
type preStateCommitHookParams struct {
	header *types.Header
	txn    *state.Transition
}

// verifyBlockHook checks if the block is an epoch block and if it has any transactions
func (pos *PoSMechanism) preStateCommitHook(rawParams interface{}) error {
	params, ok := rawParams.(*preStateCommitHookParams)
	if !ok {
		return ErrInvalidHookParam
	}

	// Deploy Staking contract
	contractState, err := stakingHelper.PredeployStakingSC(nil, stakingHelper.PredeployParams{
		MinValidatorCount: pos.MinValidatorCount,
		MaxValidatorCount: pos.MaxValidatorCount,
	})
	if err != nil {
		return err
	}

	if err := params.txn.SetAccountDirectly(staking.AddrStakingContract, contractState); err != nil {
		return err
	}

	return nil
}

// initializeHookMap registers the hooks that the PoS mechanism
// should have
func (pos *PoSMechanism) initializeHookMap() {
	// Create the hook map
	pos.hookMap = make(map[HookType]func(interface{}) error)

	// Register the AcceptStateLogHook
	pos.hookMap[AcceptStateLogHook] = pos.acceptStateLogHook

	// Register the InsertBlockHook
	pos.hookMap[InsertBlockHook] = pos.insertBlockHook

	// Register the VerifyBlockHook
	pos.hookMap[VerifyBlockHook] = pos.verifyBlockHook

	// Register the PreStateCommitHook
	pos.hookMap[PreStateCommitHook] = pos.preStateCommitHook

	// Register the CalculateProposerHook
	pos.hookMap[CalculateProposerHook] = pos.calculateProposerHook
}

// ShouldWriteTransactions indicates if transactions should be written to a block
func (pos *PoSMechanism) ShouldWriteTransactions(blockNumber uint64) bool {
	// Epoch blocks should be empty
	return pos.IsInRange(blockNumber) && !pos.ibft.IsLastOfEpoch(blockNumber)
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
