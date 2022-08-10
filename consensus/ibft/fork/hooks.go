package fork

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/0xPolygon/polygon-edge/validators/valset"
)

func registerPoSHook(
	hooks *hook.HookManager,
	epochSize uint64,
) {
	isLastEpoch := func(height uint64) bool {
		return height > 0 && height%epochSize == 0
	}

	hooks.ShouldWriteTransactionFunc = func(height uint64) bool {
		return !isLastEpoch(height)
	}

	hooks.VerifyBlockFunc = func(block *types.Block) error {
		if isLastEpoch(block.Number()) && len(block.Transactions) > 0 {
			return fmt.Errorf("block can't have transactions in the last of epoch")
		}

		return nil
	}
}

func registerValidatorSetHook(
	hooks *hook.HookManager,
	set valset.ValidatorSet,
) {
	if hm, ok := set.(valset.HeaderModifier); ok {
		hooks.ModifyHeaderFunc = hm.ModifyHeader
		hooks.VerifyHeaderFunc = hm.VerifyHeader
	}

	if ph, ok := set.(valset.HeaderProcessor); ok {
		hooks.ProcessHeaderFunc = ph.ProcessHeader
	}
}

func registerUpdateValidatorSetHook(
	hooks *hook.HookManager,
	set valset.ValidatorSet,
	newValidators validators.Validators,
	from uint64,
) {
	if us, ok := set.(valset.Updatable); ok {
		hooks.PostInsertBlockFunc = func(b *types.Block) error {
			return us.UpdateSet(newValidators, from)
		}
	}
}

func registerContractDeploymentHook(
	hooks *hook.HookManager,
	fork *IBFTFork,
) {
	hooks.PreCommitStateFunc = func(header *types.Header, txn *state.Transition) error {
		contractState, err := stakingHelper.PredeployStakingSC(fork.Validators, stakingHelper.PredeployParams{
			MinValidatorCount: fork.MinValidatorCount.Value,
			MaxValidatorCount: fork.MaxValidatorCount.Value,
		})

		if err != nil {
			return err
		}

		return txn.SetAccountDirectly(staking.AddrStakingContract, contractState)
	}
}
