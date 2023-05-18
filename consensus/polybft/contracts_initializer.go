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
		NewStateSender:      contracts.L2StateSenderContract,
		NewStateReceiver:    contracts.StateReceiverContract,
		NewRootChainManager: polyBFTConfig.Bridge.CustomSupernetManagerAddr,
		NewEpochSize:        new(big.Int).SetUint64(polyBFTConfig.EpochSize),
		InitalValidators:    initialValidators,
	}

	return initFn.EncodeAbi()
}

// getInitRewardPoolInput builds input parameters for RewardPool SC initialization
func getInitRewardPoolInput(polybftConfig PolyBFTConfig) ([]byte, error) {
	initFn := &contractsapi.InitializeRewardPoolFn{
		NewRewardToken:  polybftConfig.RewardConfig.TokenAddress,
		NewRewardWallet: polybftConfig.RewardConfig.WalletAddress,
		NewValidatorSet: contracts.ValidatorSetContract,
		NewBaseReward:   new(big.Int).SetUint64(polybftConfig.EpochReward),
	}

	return initFn.EncodeAbi()
}

// getInitERC20PredicateInput builds initialization input parameters for child chain ERC20Predicate SC
func getInitERC20PredicateInput(config *BridgeConfig, childOriginatedTokens bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childOriginatedTokens {
		params = &contractsapi.InitializeRootMintableERC20PredicateFn{
			NewL2StateSender:       contracts.L2StateSenderContract,
			NewStateReceiver:       contracts.StateReceiverContract,
			NewChildERC20Predicate: config.ChildMintableERC20PredicateAddr,
			NewChildTokenTemplate:  contracts.ChildERC20Contract,
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

// getInitChildERC20PredicateAccessListInput builds input parameters for ChildERC20PredicateAccessList SC initialization
func getInitChildERC20PredicateAccessListInput(config *BridgeConfig, owner types.Address) ([]byte, error) {
	params := &contractsapi.InitializeChildERC20PredicateAccessListFn{
		NewL2StateSender:          contracts.L2StateSenderContract,
		NewStateReceiver:          contracts.StateReceiverContract,
		NewRootERC20Predicate:     config.RootERC20PredicateAddr,
		NewChildTokenTemplate:     contracts.ChildERC20Contract,
		NewNativeTokenRootAddress: config.RootNativeERC20Addr,
		NewUseAllowList:           owner != contracts.SystemCaller,
		NewUseBlockList:           owner != contracts.SystemCaller,
		NewOwner:                  owner,
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
			NewChildTokenTemplate:   contracts.ChildERC20Contract,
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

// getInitChildERC721PredicateAccessListInput builds input parameters
// for ChildERC721PredicateAccessList SC initialization
func getInitChildERC721PredicateAccessListInput(config *BridgeConfig, owner types.Address) ([]byte, error) {
	params := &contractsapi.InitializeChildERC721PredicateAccessListFn{
		NewL2StateSender:       contracts.L2StateSenderContract,
		NewStateReceiver:       contracts.StateReceiverContract,
		NewRootERC721Predicate: config.RootERC721PredicateAddr,
		NewChildTokenTemplate:  contracts.ChildERC721Contract,
		NewUseAllowList:        owner != contracts.SystemCaller,
		NewUseBlockList:        owner != contracts.SystemCaller,
		NewOwner:               owner,
	}

	return params.EncodeAbi()
}

// getInitERC1155PredicateInput builds initialization input parameters for child chain ERC1155Predicate SC
func getInitERC1155PredicateInput(config *BridgeConfig, childOriginatedTokens bool) ([]byte, error) {
	var params contractsapi.StateTransactionInput
	if childOriginatedTokens {
		params = &contractsapi.InitializeRootMintableERC1155PredicateFn{
			NewL2StateSender:         contracts.L2StateSenderContract,
			NewStateReceiver:         contracts.StateReceiverContract,
			NewChildERC1155Predicate: config.ChildMintableERC1155PredicateAddr,
			NewChildTokenTemplate:    contracts.ChildERC1155Contract,
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

// getInitChildERC1155PredicateAccessListInput builds input parameters
// for ChildERC1155PredicateAccessList SC initialization
func getInitChildERC1155PredicateAccessListInput(config *BridgeConfig, owner types.Address) ([]byte, error) {
	params := &contractsapi.InitializeChildERC1155PredicateAccessListFn{
		NewL2StateSender:        contracts.L2StateSenderContract,
		NewStateReceiver:        contracts.StateReceiverContract,
		NewRootERC1155Predicate: config.RootERC1155PredicateAddr,
		NewChildTokenTemplate:   contracts.ChildERC1155Contract,
		NewUseAllowList:         owner != contracts.SystemCaller,
		NewUseBlockList:         owner != contracts.SystemCaller,
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
	result := transition.Call2(from, to, input, big.NewInt(0), 100_000_000)
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
