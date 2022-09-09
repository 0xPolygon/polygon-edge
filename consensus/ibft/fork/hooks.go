package fork

import (
	"errors"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/store"
)

var (
	ErrTxInLastEpochOfBlock = errors.New("block must not have transactions in the last of epoch")
)

// HeaderModifier is an interface for the struct that modifies block header for additional process
type HeaderModifier interface {
	ModifyHeader(*types.Header, types.Address) error
	VerifyHeader(*types.Header) error
	ProcessHeader(*types.Header) error
}

// registerHeaderModifierHooks registers hooks to modify header by validator store
func registerHeaderModifierHooks(
	hooks *hook.Hooks,
	validatorStore store.ValidatorStore,
) {
	if modifier, ok := validatorStore.(HeaderModifier); ok {
		hooks.ModifyHeaderFunc = modifier.ModifyHeader
		hooks.VerifyHeaderFunc = modifier.VerifyHeader
		hooks.ProcessHeaderFunc = modifier.ProcessHeader
	}
}

// Updatable is an interface for the struct that updates validators in the middle
type Updatable interface {
	// UpdateValidatorSet updates validators forcibly
	// in order that new validators are available from the given height
	UpdateValidatorSet(validators.Validators, uint64) error
}

// registerUpdateValidatorsHooks registers hooks to update validators in the middle
func registerUpdateValidatorsHooks(
	hooks *hook.Hooks,
	validatorStore store.ValidatorStore,
	validators validators.Validators,
	fromHeight uint64,
) {
	if us, ok := validatorStore.(Updatable); ok {
		hooks.PostInsertBlockFunc = func(b *types.Block) error {
			if fromHeight != b.Number()+1 {
				return nil
			}

			// update validators if the block height is the one before beginning height
			return us.UpdateValidatorSet(validators, fromHeight)
		}
	}
}

// registerPoSVerificationHooks registers that hooks to prevent the last epoch block from having transactions
func registerTxInclusionGuardHooks(hooks *hook.Hooks, epochSize uint64) {
	isLastEpoch := func(height uint64) bool {
		return height > 0 && height%epochSize == 0
	}

	hooks.ShouldWriteTransactionFunc = func(height uint64) bool {
		return !isLastEpoch(height)
	}

	hooks.VerifyBlockFunc = func(block *types.Block) error {
		if isLastEpoch(block.Number()) && len(block.Transactions) > 0 {
			return ErrTxInLastEpochOfBlock
		}

		return nil
	}
}

// registerStakingContractDeploymentHooks registers hooks
// to deploy or update staking contract
func registerStakingContractDeploymentHooks(
	hooks *hook.Hooks,
	fork *IBFTFork,
) {
	hooks.PreCommitStateFunc = func(header *types.Header, txn *state.Transition) error {
		// safe check
		if header.Number != fork.Deployment.Value {
			return nil
		}

		if txn.AccountExists(staking.AddrStakingContract) {
			// update bytecode of deployed contract
			codeBytes, err := hex.DecodeHex(stakingHelper.StakingSCBytecode)
			if err != nil {
				return err
			}

			return txn.SetCodeDirectly(staking.AddrStakingContract, codeBytes)
		} else {
			// deploy contract
			contractState, err := stakingHelper.PredeployStakingSC(
				fork.Validators,
				getPreDeployParams(fork),
			)

			if err != nil {
				return err
			}

			return txn.SetAccountDirectly(staking.AddrStakingContract, contractState)
		}
	}
}

// getPreDeployParams returns PredeployParams for Staking Contract from IBFTFork
func getPreDeployParams(fork *IBFTFork) stakingHelper.PredeployParams {
	params := stakingHelper.PredeployParams{
		MinValidatorCount: stakingHelper.MinValidatorCount,
		MaxValidatorCount: stakingHelper.MaxValidatorCount,
	}

	if fork.MinValidatorCount != nil {
		params.MinValidatorCount = fork.MinValidatorCount.Value
	}

	if fork.MaxValidatorCount != nil {
		params.MaxValidatorCount = fork.MaxValidatorCount.Value
	}

	return params
}
