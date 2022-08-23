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

// registerPoSHook registers additional processes for PoS
func registerPoSHook(
	hooks *hook.Hooks,
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
			return ErrTxInLastEpochOfBlock
		}

		return nil
	}
}

// registerValidatorStoreHook registers additional processes
// for the ValidatorStore that modifies header
func registerValidatorStoreHook(
	hooks *hook.Hooks,
	set store.ValidatorStore,
) {
	if hm, ok := set.(store.HeaderModifier); ok {
		hooks.ModifyHeaderFunc = hm.ModifyHeader
		hooks.VerifyHeaderFunc = hm.VerifyHeader
	}

	if ph, ok := set.(store.HeaderProcessor); ok {
		hooks.ProcessHeaderFunc = ph.ProcessHeader
	}
}

// registerUpdateValidatorStoreHook registers additional process
// to update validators at specified height
func registerUpdateValidatorStoreHook(
	hooks *hook.Hooks,
	set store.ValidatorStore,
	newValidators validators.Validators,
	height uint64,
) {
	if us, ok := set.(store.Updatable); ok {
		hooks.PostInsertBlockFunc = func(b *types.Block) error {
			return us.UpdateSet(newValidators, height)
		}
	}
}

// registerContractDeploymentHook registers additional process
// to deploy contract or update contract byte code
func registerContractDeploymentHook(
	hooks *hook.Hooks,
	fork *IBFTFork,
) {
	hooks.PreCommitStateFunc = func(header *types.Header, txn *state.Transition) error {
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
