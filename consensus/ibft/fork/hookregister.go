package fork

import (
	"github.com/0xPolygon/polygon-edge/consensus/ibft/hook"
)

// PoAHookRegisterer that registers hooks for PoA mode
type PoAHookRegister struct {
	getValidatorsStore    func(*IBFTFork) ValidatorStore
	poaForks              IBFTForks
	updateValidatorsForks map[uint64]*IBFTFork
}

// NewPoAHookRegisterer is a constructor of PoAHookRegister
func NewPoAHookRegisterer(
	getValidatorsStore func(*IBFTFork) ValidatorStore,
	forks IBFTForks,
) *PoAHookRegister {
	poaForks := forks.filterByType(PoA)

	updateValidatorsForks := make(map[uint64]*IBFTFork)

	for _, fork := range poaForks {
		if fork.Validators == nil {
			continue
		}

		updateValidatorsForks[fork.From.Value] = fork
	}

	return &PoAHookRegister{
		getValidatorsStore:    getValidatorsStore,
		poaForks:              poaForks,
		updateValidatorsForks: updateValidatorsForks,
	}
}

// RegisterHooks registers hooks of PoA for voting and validators updating
func (r *PoAHookRegister) RegisterHooks(hooks *hook.Hooks, height uint64) {
	if currentFork := r.poaForks.getFork(height); currentFork != nil {
		// in PoA mode currently
		validatorStore := r.getValidatorsStore(currentFork)

		registerHeaderModifierHooks(hooks, validatorStore)
	}

	// update validators in the end of the last block
	if updateValidatorsFork, ok := r.updateValidatorsForks[height+1]; ok {
		validatorStore := r.getValidatorsStore(updateValidatorsFork)

		registerUpdateValidatorsHooks(
			hooks,
			validatorStore,
			updateValidatorsFork.Validators,
			updateValidatorsFork.From.Value,
		)
	}
}

// PoAHookRegisterer that registers hooks for PoS mode
type PoSHookRegister struct {
	posForks            IBFTForks
	epochSize           uint64
	deployContractForks map[uint64]*IBFTFork
}

// NewPoSHookRegister is a constructor of PoSHookRegister
func NewPoSHookRegister(
	forks IBFTForks,
	epochSize uint64,
) *PoSHookRegister {
	posForks := forks.filterByType(PoS)

	deployContractForks := make(map[uint64]*IBFTFork)

	for _, fork := range posForks {
		if fork.Deployment == nil {
			continue
		}

		deployContractForks[fork.Deployment.Value] = fork
	}

	return &PoSHookRegister{
		posForks:            posForks,
		epochSize:           epochSize,
		deployContractForks: deployContractForks,
	}
}

// RegisterHooks registers hooks of PoA for additional block verification and contract deployment
func (r *PoSHookRegister) RegisterHooks(hooks *hook.Hooks, height uint64) {
	if currentFork := r.posForks.getFork(height); currentFork != nil {
		// in PoS mode currently
		registerTxInclusionGuardHooks(hooks, r.epochSize)
	}

	if deploymentFork, ok := r.deployContractForks[height]; ok {
		// deploy or update staking contract in deployment height
		registerStakingContractDeploymentHooks(hooks, deploymentFork)
	}
}
