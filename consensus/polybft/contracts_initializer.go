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
	contractCallGasLimit = 100_000_000
)

// initValidatorSet initializes ValidatorSet SC
func initValidatorSet(polyBFTConfig PolyBFTConfig, transition *state.Transition) error {
	initialValidators := make([]*contractsapi.ValidatorInit, len(polyBFTConfig.InitialValidatorSet))
	for i, validator := range polyBFTConfig.InitialValidatorSet {
		initialValidators[i] = &contractsapi.ValidatorInit{
			Addr:  validator.Address,
			Stake: validator.Stake,
		}
	}

	initFn := &contractsapi.InitializeValidatorSetFn{
		NewStateSender:      contracts.L2StateSenderContract,
		NewStateReceiver:    contracts.StateReceiverContract,
		NewRootChainManager: polyBFTConfig.Bridge.CustomSupernetManagerAddr,
		NewEpochSize:        new(big.Int).SetUint64(polyBFTConfig.EpochSize),
		InitialValidators:   initialValidators,
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("ValidatorSet.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.ValidatorSetContract, input, "ValidatorSet.initialize", transition)
}

// initRewardPool initializes RewardPool SC
func initRewardPool(polybftConfig PolyBFTConfig, transition *state.Transition) error {
	initFn := &contractsapi.InitializeRewardPoolFn{
		NewRewardToken:  polybftConfig.RewardConfig.TokenAddress,
		NewRewardWallet: polybftConfig.RewardConfig.WalletAddress,
		NewValidatorSet: contracts.ValidatorSetContract,
		NewBaseReward:   new(big.Int).SetUint64(polybftConfig.EpochReward),
	}

	input, err := initFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("RewardPool.initialize params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller,
		contracts.RewardPoolContract, input, "RewardPool.initialize", transition)
}

// getInitERC20PredicateInput builds initialization input parameters for child chain ERC20Predicate SC
func getInitERC20PredicateInput(config *BridgeConfig, childChainMintable bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childChainMintable {
		params = &contractsapi.InitializeRootMintableERC20PredicateFn{
			NewL2StateSender:       contracts.L2StateSenderContract,
			NewStateReceiver:       contracts.StateReceiverContract,
			NewChildERC20Predicate: config.ChildMintableERC20PredicateAddr,
			NewChildTokenTemplate:  config.ChildERC20Addr,
		}
	} else {
		params = &contractsapi.InitializeChildERC20PredicateFn{
			NewL2StateSender:          contracts.L2StateSenderContract,
			NewStateReceiver:          contracts.StateReceiverContract,
			NewRootERC20Predicate:     config.RootERC20PredicateAddr,
			NewChildTokenTemplate:     contracts.ChildERC20Contract,
			NewNativeTokenRootAddress: config.RootNativeERC20Addr,
		}
	}

	return params.EncodeAbi()
}

// getInitERC20PredicateACLInput builds initialization input parameters for child chain ERC20PredicateAccessList SC
func getInitERC20PredicateACLInput(config *BridgeConfig, owner types.Address,
	useAllowList, useBlockList, childChainMintable bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childChainMintable {
		params = &contractsapi.InitializeRootMintableERC20PredicateACLFn{
			NewL2StateSender:       contracts.L2StateSenderContract,
			NewStateReceiver:       contracts.StateReceiverContract,
			NewChildERC20Predicate: config.ChildMintableERC20PredicateAddr,
			NewChildTokenTemplate:  config.ChildERC20Addr,
			NewUseAllowList:        useAllowList,
			NewUseBlockList:        useBlockList,
			NewOwner:               owner,
		}
	} else {
		params = &contractsapi.InitializeChildERC20PredicateACLFn{
			NewL2StateSender:          contracts.L2StateSenderContract,
			NewStateReceiver:          contracts.StateReceiverContract,
			NewRootERC20Predicate:     config.RootERC20PredicateAddr,
			NewChildTokenTemplate:     contracts.ChildERC20Contract,
			NewNativeTokenRootAddress: config.RootNativeERC20Addr,
			NewUseAllowList:           useAllowList,
			NewUseBlockList:           useBlockList,
			NewOwner:                  owner,
		}
	}

	return params.EncodeAbi()
}

// getInitERC721PredicateInput builds initialization input parameters for child chain ERC721Predicate SC
func getInitERC721PredicateInput(config *BridgeConfig, childOriginatedTokens bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childOriginatedTokens {
		params = &contractsapi.InitializeRootMintableERC721PredicateFn{
			NewL2StateSender:        contracts.L2StateSenderContract,
			NewStateReceiver:        contracts.StateReceiverContract,
			NewChildERC721Predicate: config.ChildMintableERC721PredicateAddr,
			NewChildTokenTemplate:   config.ChildERC721Addr,
		}
	} else {
		params = &contractsapi.InitializeChildERC721PredicateFn{
			NewL2StateSender:       contracts.L2StateSenderContract,
			NewStateReceiver:       contracts.StateReceiverContract,
			NewRootERC721Predicate: config.RootERC721PredicateAddr,
			NewChildTokenTemplate:  contracts.ChildERC721Contract,
		}
	}

	return params.EncodeAbi()
}

// getInitERC721PredicateACLInput builds initialization input parameters
// for child chain ERC721PredicateAccessList SC
func getInitERC721PredicateACLInput(config *BridgeConfig, owner types.Address,
	useAllowList, useBlockList, childChainMintable bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childChainMintable {
		params = &contractsapi.InitializeRootMintableERC721PredicateACLFn{
			NewL2StateSender:        contracts.L2StateSenderContract,
			NewStateReceiver:        contracts.StateReceiverContract,
			NewChildERC721Predicate: config.ChildMintableERC721PredicateAddr,
			NewChildTokenTemplate:   config.ChildERC721Addr,
			NewUseAllowList:         useAllowList,
			NewUseBlockList:         useBlockList,
			NewOwner:                owner,
		}
	} else {
		params = &contractsapi.InitializeChildERC721PredicateACLFn{
			NewL2StateSender:       contracts.L2StateSenderContract,
			NewStateReceiver:       contracts.StateReceiverContract,
			NewRootERC721Predicate: config.RootERC721PredicateAddr,
			NewChildTokenTemplate:  contracts.ChildERC721Contract,
			NewUseAllowList:        useAllowList,
			NewUseBlockList:        useBlockList,
			NewOwner:               owner,
		}
	}

	return params.EncodeAbi()
}

// getInitERC1155PredicateInput builds initialization input parameters for child chain ERC1155Predicate SC
func getInitERC1155PredicateInput(config *BridgeConfig, childChainMintable bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childChainMintable {
		params = &contractsapi.InitializeRootMintableERC1155PredicateFn{
			NewL2StateSender:         contracts.L2StateSenderContract,
			NewStateReceiver:         contracts.StateReceiverContract,
			NewChildERC1155Predicate: config.ChildMintableERC1155PredicateAddr,
			NewChildTokenTemplate:    config.ChildERC1155Addr,
		}
	} else {
		params = &contractsapi.InitializeChildERC1155PredicateFn{
			NewL2StateSender:        contracts.L2StateSenderContract,
			NewStateReceiver:        contracts.StateReceiverContract,
			NewRootERC1155Predicate: config.RootERC1155PredicateAddr,
			NewChildTokenTemplate:   contracts.ChildERC1155Contract,
		}
	}

	return params.EncodeAbi()
}

// getInitERC1155PredicateACLInput builds initialization input parameters
// for child chain ERC1155PredicateAccessList SC
func getInitERC1155PredicateACLInput(config *BridgeConfig, owner types.Address,
	useAllowList, useBlockList, childChainMintable bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childChainMintable {
		params = &contractsapi.InitializeRootMintableERC1155PredicateACLFn{
			NewL2StateSender:         contracts.L2StateSenderContract,
			NewStateReceiver:         contracts.StateReceiverContract,
			NewChildERC1155Predicate: config.ChildMintableERC1155PredicateAddr,
			NewChildTokenTemplate:    config.ChildERC1155Addr,
			NewUseAllowList:          useAllowList,
			NewUseBlockList:          useBlockList,
			NewOwner:                 owner,
		}
	} else {
		params = &contractsapi.InitializeChildERC1155PredicateACLFn{
			NewL2StateSender:        contracts.L2StateSenderContract,
			NewStateReceiver:        contracts.StateReceiverContract,
			NewRootERC1155Predicate: config.RootERC1155PredicateAddr,
			NewChildTokenTemplate:   contracts.ChildERC1155Contract,
			NewUseAllowList:         useAllowList,
			NewUseBlockList:         useBlockList,
			NewOwner:                owner,
		}
	}

	return params.EncodeAbi()
}

// mintRewardTokensToWallet mints configured amount of reward tokens to reward wallet address
func mintRewardTokensToWallet(polyBFTConfig PolyBFTConfig, transition *state.Transition) error {
	if isNativeRewardToken(polyBFTConfig) {
		// if reward token is a native erc20 token, we don't need to mint an amount of tokens
		// for given wallet address to it since this is done in premine
		return nil
	}

	mintFn := contractsapi.MintRootERC20Fn{
		To:     polyBFTConfig.RewardConfig.WalletAddress,
		Amount: polyBFTConfig.RewardConfig.WalletAmount,
	}

	input, err := mintFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("RewardToken.mint params encoding failed: %w", err)
	}

	return callContract(contracts.SystemCaller, polyBFTConfig.RewardConfig.TokenAddress, input,
		"RewardToken.mint", transition)
}

// approveRewardPoolAsSpender approves reward pool contract as reward token spender
// since reward pool distributes rewards.
func approveRewardPoolAsSpender(polyBFTConfig PolyBFTConfig, transition *state.Transition) error {
	approveFn := &contractsapi.ApproveRootERC20Fn{
		Spender: contracts.RewardPoolContract,
		Amount:  polyBFTConfig.RewardConfig.WalletAmount,
	}

	input, err := approveFn.EncodeAbi()
	if err != nil {
		return fmt.Errorf("RewardToken.approve params encoding failed: %w", err)
	}

	return callContract(polyBFTConfig.RewardConfig.WalletAddress,
		polyBFTConfig.RewardConfig.TokenAddress, input, "RewardToken.approve", transition)
}

// callContract calls given smart contract function, encoded in input parameter
func callContract(from, to types.Address, input []byte, contractName string, transition *state.Transition) error {
	result := transition.Call2(from, to, input, big.NewInt(0), contractCallGasLimit)
	if result.Failed() {
		if result.Reverted() {
			if revertReason, err := abi.UnpackRevertError(result.ReturnValue); err == nil {
				return fmt.Errorf("%s contract call was reverted: %s", contractName, revertReason)
			}
		}

		return fmt.Errorf("%s contract call failed: %w", contractName, result.Err)
	}

	return nil
}

// isNativeRewardToken returns true in case a native token is used as a reward token as well
func isNativeRewardToken(cfg PolyBFTConfig) bool {
	return cfg.RewardConfig.TokenAddress == contracts.NativeERC20TokenContract
}
