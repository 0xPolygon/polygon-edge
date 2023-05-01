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

// getInitChildValidatorSetInput builds input parameters for ChildValidatorSet SC initialization
func getInitChildValidatorSetInput(polyBFTConfig PolyBFTConfig) ([]byte, error) {
	apiValidators := make([]*contractsapi.ValidatorInit, len(polyBFTConfig.InitialValidatorSet))

	for i, validator := range polyBFTConfig.InitialValidatorSet {
		validatorData, err := validator.ToValidatorInitAPIBinding()
		if err != nil {
			return nil, err
		}

		apiValidators[i] = validatorData
	}

	params := &contractsapi.InitializeChildValidatorSetFn{
		Init: &contractsapi.InitStruct{
			EpochReward:   new(big.Int).SetUint64(polyBFTConfig.EpochReward),
			MinStake:      big.NewInt(minStake),
			MinDelegation: big.NewInt(minDelegation),
			EpochSize:     new(big.Int).SetUint64(polyBFTConfig.EpochSize),
		},
		NewBls:     contracts.BLSContract,
		Governance: polyBFTConfig.Governance,
		Validators: apiValidators,
	}

	return params.EncodeAbi()
}

// getInitChildERC20PredicateInput builds input parameters for ChildERC20Predicate SC initialization
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

	if config.Bridge != nil {
		rootERC20PredicateAddr = config.Bridge.RootERC20PredicateAddr
		rootERC20Addr = config.Bridge.RootNativeERC20Addr
	}

	params := &contractsapi.InitializeChildERC20PredicateAccessListFn{
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

// getInitChildERC721PredicateAccessListInput builds input parameters for ChildERC721PredicateAccessList SC initialization
func getInitChildERC721PredicateAccessListInput(config PolyBFTConfig) ([]byte, error) {
	rootERC721PredicateAccessListAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config.Bridge != nil {
		rootERC721PredicateAccessListAddr = config.Bridge.RootERC721PredicateAddr
	}

	params := &contractsapi.InitializeChildERC721PredicateAccessListFn{
		NewL2StateSender:       contracts.L2StateSenderContract,
		NewStateReceiver:       contracts.StateReceiverContract,
		NewRootERC721Predicate: rootERC721PredicateAccessListAddr,
		NewChildTokenTemplate:  contracts.ChildERC721Contract,
		UseAllowList:           config.BridgeAllowListActive,
		UseBlockList:           config.BridgeBlockListActive,
		NewOwner:               contracts.SystemCaller,
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

// getInitChildERC1155PredicateAccessListInput builds input parameters for ChildERC1155PredicateAccessList SC initialization
func getInitChildERC1155PredicateAccessListInput(config PolyBFTConfig) ([]byte, error) {
	rootERC1155PredicateAccessListAddr := types.StringToAddress(disabledBridgeRootPredicateAddr)

	if config.Bridge != nil {
		rootERC1155PredicateAccessListAddr = config.Bridge.RootERC1155PredicateAddr
	}

	params := &contractsapi.InitializeChildERC1155PredicateAccessListFn{
		NewL2StateSender:        contracts.L2StateSenderContract,
		NewStateReceiver:        contracts.StateReceiverContract,
		NewRootERC1155Predicate: rootERC1155PredicateAccessListAddr,
		NewChildTokenTemplate:   contracts.ChildERC1155Contract,
		UseAllowList:            config.BridgeAllowListActive,
		UseBlockList:            config.BridgeBlockListActive,
		NewOwner:                contracts.SystemCaller,
	}

	return params.EncodeAbi()
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
