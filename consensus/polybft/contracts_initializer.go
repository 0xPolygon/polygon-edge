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

// getInitRewardPoolInput builds input parameters for RewardPool SC initialization
func getInitRewardPoolInput(polybftConfig PolyBFTConfig) ([]byte, error) {
	initFn := &contractsapi.InitializeRewardPoolFn{
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

// getInitChildERC20PredicateAccessListInput builds input parameters for ChildERC20PredicateAccessList SC initialization
func getInitChildERC20PredicateAccessListInput(config PolyBFTConfig) ([]byte, error) {
	//nolint:godox
	// to be fixed with EVM-541
	// TODO: @Stefan-Ethernal Temporary workaround just to be able to run cluster in non-bridge mode, until SC is fixed
	rootERC20PredicateAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)
	rootERC20Addr := types.ZeroAddress

	//nolint:godox
	// TODO: This can be removed as we'll always have a bridge config
	if config.Bridge != nil {
		rootERC20PredicateAddr = config.Bridge.RootERC20PredicateAddr
		rootERC20Addr = config.Bridge.RootNativeERC20Addr
	}

	// The owner of the contract will be the allow list admin or the block list admin, if any of them is set.
	owner := contracts.SystemCaller
	if config.BridgeAllowListAdmin != types.ZeroAddress {
		owner = config.BridgeAllowListAdmin
	} else if config.BridgeBlockListAdmin != types.ZeroAddress {
		owner = config.BridgeBlockListAdmin
	}

	params := &contractsapi.InitializeChildERC20PredicateAccessListFn{
		NewL2StateSender:          contracts.L2StateSenderContract,
		NewStateReceiver:          contracts.StateReceiverContract,
		NewRootERC20Predicate:     rootERC20PredicateAddr,
		NewChildTokenTemplate:     contracts.ChildERC20Contract,
		NewNativeTokenRootAddress: rootERC20Addr,
		UseAllowList:              config.BridgeAllowListAdmin != types.ZeroAddress,
		UseBlockList:              config.BridgeBlockListAdmin != types.ZeroAddress,
		NewOwner:                  owner,
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

// getInitChildERC721PredicateAccessListInput builds input parameters
// for ChildERC721PredicateAccessList SC initialization
func getInitChildERC721PredicateAccessListInput(config PolyBFTConfig) ([]byte, error) {
	rootERC721PredicateAccessListAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config.Bridge != nil {
		rootERC721PredicateAccessListAddr = config.Bridge.RootERC721PredicateAddr
	}

	// The owner of the contract will be the allow list admin or the block list admin, if any of them is set.
	owner := contracts.SystemCaller
	if config.BridgeAllowListAdmin != types.ZeroAddress {
		owner = config.BridgeAllowListAdmin
	} else if config.BridgeBlockListAdmin != types.ZeroAddress {
		owner = config.BridgeBlockListAdmin
	}

	params := &contractsapi.InitializeChildERC721PredicateAccessListFn{
		NewL2StateSender:       contracts.L2StateSenderContract,
		NewStateReceiver:       contracts.StateReceiverContract,
		NewRootERC721Predicate: rootERC721PredicateAccessListAddr,
		NewChildTokenTemplate:  contracts.ChildERC721Contract,
		UseAllowList:           config.BridgeAllowListAdmin != types.ZeroAddress,
		UseBlockList:           config.BridgeBlockListAdmin != types.ZeroAddress,
		NewOwner:               owner,
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

// getInitChildERC1155PredicateAccessListInput builds input parameters
// for ChildERC1155PredicateAccessList SC initialization
func getInitChildERC1155PredicateAccessListInput(config PolyBFTConfig) ([]byte, error) {
	rootERC1155PredicateAccessListAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config.Bridge != nil {
		rootERC1155PredicateAccessListAddr = config.Bridge.RootERC1155PredicateAddr
	}

	// The owner of the contract will be the allow list admin or the block list admin, if any of them is set.
	owner := contracts.SystemCaller
	if config.BridgeAllowListAdmin != types.ZeroAddress {
		owner = config.BridgeAllowListAdmin
	} else if config.BridgeBlockListAdmin != types.ZeroAddress {
		owner = config.BridgeBlockListAdmin
	}

	params := &contractsapi.InitializeChildERC1155PredicateAccessListFn{
		NewL2StateSender:        contracts.L2StateSenderContract,
		NewStateReceiver:        contracts.StateReceiverContract,
		NewRootERC1155Predicate: rootERC1155PredicateAccessListAddr,
		NewChildTokenTemplate:   contracts.ChildERC1155Contract,
		UseAllowList:            config.BridgeAllowListAdmin != types.ZeroAddress,
		UseBlockList:            config.BridgeBlockListAdmin != types.ZeroAddress,
		NewOwner:                owner,
	}

	return params.EncodeAbi()
}

// mintRewardTokensToWalletAddress mints configured amount of reward tokens to reward wallet address
func mintRewardTokensToWalletAddress(polyBFTConfig *PolyBFTConfig, transition *state.Transition) error {
	approveFn := &contractsapi.ApproveRootERC20Fn{
		Spender: contracts.RewardPoolContract,
		Amount:  polyBFTConfig.RewardConfig.WalletAmount,
	}

	input, err := approveFn.EncodeAbi()
	if err != nil {
		return err
	}

	if err = initContract(polyBFTConfig.RewardConfig.WalletAddress,
		polyBFTConfig.RewardConfig.TokenAddress, input, "RewardToken", transition); err != nil {
		return err
	}

	if polyBFTConfig.RewardConfig.TokenAddress == contracts.NativeERC20TokenContract {
		// if reward token is a native erc20 token, we don't need to mint an amount of tokens
		// for given wallet address to it since this is done in premine
		return nil
	}

	mintFn := abi.MustNewMethod("function mint(address, uint256)")

	input, err = mintFn.Encode([]interface{}{polyBFTConfig.RewardConfig.WalletAddress,
		polyBFTConfig.RewardConfig.WalletAmount})
	if err != nil {
		return err
	}

	return initContract(contracts.SystemCaller, polyBFTConfig.RewardConfig.TokenAddress, input, "RewardToken", transition)
}

func initContract(from, to types.Address, input []byte, contractName string, transition *state.Transition) error {
	result := transition.Call2(from, to, input,
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
