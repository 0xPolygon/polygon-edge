package polybft

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

const (
	// safe numbers for the test
	minStake      = 1
	minDelegation = 1

	disabledBridgeRootPredicateAddr = "0xDEAD"
)

// getInitValidatorSetInput builds input parameters for ValidatorSet SC initialization
func getInitValidatorSetInput(polyBFTConfig PolyBFTConfig) ([]byte, error) {
	initialValidators := make([]*contractsapi.ValidatorInit, len(polyBFTConfig.InitialValidatorSet))
	for i, validator := range polyBFTConfig.InitialValidatorSet {
		initialValidators[i] = &contractsapi.ValidatorInit{
			Addr:  validator.Address,
			Stake: validator.Stake,
		}
	}

	initFn := &contractsapi.InitializeValidatorSetFn{
		StateSender:      contracts.L2StateSenderContract,
		StateReceiver:    contracts.StateReceiverContract,
		RootChainManager: polyBFTConfig.Bridge.CustomSupernetManagerAddr,
		EpochSize_:       new(big.Int).SetUint64(polyBFTConfig.EpochSize),
		InitalValidators: initialValidators,
	}

	return initFn.EncodeAbi()
}

// getInitRewardDistributorInput builds input parameters for RewardDistributor SC initialization
func getInitRewardDistributorInput(polybftConfig PolyBFTConfig) ([]byte, error) {
	initFn := &contractsapi.InitializeRewardDistributorFn{
		RewardToken:  polybftConfig.RewardConfig.TokenAddress,
		RewardWallet: polybftConfig.RewardConfig.WalletAddress,
		ValidatorSet: contracts.ValidatorSetContract,
		BaseReward:   new(big.Int).SetUint64(polybftConfig.EpochReward),
	}

	return initFn.EncodeAbi()
}

// getInitChildERC20PredicateInput builds input parameters for ERC20Predicate SC initialization
func getInitChildERC20PredicateInput(config *BridgeConfig) ([]byte, error) {
	//nolint:godox
	// to be fixed with EVM-541
	// TODO: @Stefan-Ethernal Temporary workaround just to be able to run cluster in non-bridge mode, until SC is fixed
	rootERC20PredicateAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)
	rootERC20Addr := types.ZeroAddress

	if config != nil {
		rootERC20PredicateAddr = config.RootERC20PredicateAddr
		rootERC20Addr = config.RootNativeERC20Addr
	}

	params := &contractsapi.InitializeChildERC20PredicateFn{
		NewL2StateSender:          contracts.L2StateSenderContract,
		NewStateReceiver:          contracts.StateReceiverContract,
		NewRootERC20Predicate:     rootERC20PredicateAddr,
		NewChildTokenTemplate:     contracts.ChildERC20Contract,
		NewNativeTokenRootAddress: rootERC20Addr,
	}

	return params.EncodeAbi()
}

// getInitChildERC721PredicateInput builds input parameters for ChildERC721Predicate SC initialization
func getInitChildERC721PredicateInput(config *BridgeConfig) ([]byte, error) {
	rootERC721PredicateAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config != nil {
		rootERC721PredicateAddr = config.RootERC721PredicateAddr
	}

	params := &contractsapi.InitializeChildERC721PredicateFn{
		NewL2StateSender:       contracts.L2StateSenderContract,
		NewStateReceiver:       contracts.StateReceiverContract,
		NewRootERC721Predicate: rootERC721PredicateAddr,
		NewChildTokenTemplate:  contracts.ChildERC721Contract,
	}

	return params.EncodeAbi()
}

// getInitChildERC1155PredicateInput builds input parameters for ChildERC1155Predicate SC initialization
func getInitChildERC1155PredicateInput(config *BridgeConfig) ([]byte, error) {
	rootERC1155PredicateAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config != nil {
		rootERC1155PredicateAddr = config.RootERC1155PredicateAddr
	}

	params := &contractsapi.InitializeChildERC1155PredicateFn{
		NewL2StateSender:        contracts.L2StateSenderContract,
		NewStateReceiver:        contracts.StateReceiverContract,
		NewRootERC1155Predicate: rootERC1155PredicateAddr,
		NewChildTokenTemplate:   contracts.ChildERC1155Contract,
	}

	return params.EncodeAbi()
}

// mintRewardTokensToWalletAddress mints configured amount of reward tokens to reward wallet address
func mintRewardTokensToWalletAddress(polyBFTConfig *PolyBFTConfig, transition *state.Transition) error {
	if polyBFTConfig.RewardConfig.TokenAddress == contracts.NativeERC20TokenContract ||
		polyBFTConfig.RewardConfig.WalletAmount.Uint64() > 0 {
		// if reward token is a native erc20 token, we don't need to mint an amount of tokens
		// for given wallet address to it since this is done in premine
		return nil
	}

	approveFn := &contractsapi.ApproveRootERC20Fn{
		Spender: polyBFTConfig.RewardConfig.WalletAddress,
		Amount:  polyBFTConfig.RewardConfig.WalletAmount,
	}

	input, err := approveFn.EncodeAbi()
	if err != nil {
		return err
	}

	if err = initContract(polyBFTConfig.RewardConfig.TokenAddress, input, "RewardToken", transition); err != nil {
		return err
	}

	mintFn := &contractsapi.MintRootERC20Fn{
		To:     polyBFTConfig.RewardConfig.WalletAddress,
		Amount: polyBFTConfig.RewardConfig.WalletAmount,
	}

	input, err = mintFn.EncodeAbi()
	if err != nil {
		return err
	}

	return initContract(polyBFTConfig.RewardConfig.TokenAddress, input, "RewardToken", transition)
}

func initContract(to types.Address, input []byte, contractName string, transition *state.Transition) error {
	result := transition.Call2(contracts.SystemCaller, to, input,
		big.NewInt(0), 100_000_000)

	if result.Failed() {
		if result.Reverted() {
			unpackedRevert, err := abi.UnpackRevertError(result.ReturnValue)
			if err == nil {
				fmt.Printf("%v.initialize %v\n", contractName, unpackedRevert)
			}
		}

		return fmt.Errorf("failed to initialize %s contract. Reason: %w", contractName, result.Err)
	}

	return nil
}
