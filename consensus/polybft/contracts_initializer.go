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
)

var (
	nativeTokenName     = "Polygon"
	nativeTokenSymbol   = "MATIC"
	nativeTokenDecimals = uint8(18)
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

	params := &contractsapi.InitializeChildValidatorSetFunction{
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

// getInitChildERC20PredicateInput builds input parameters for ERC20Predicate SC initialization
func getInitChildERC20PredicateInput(config *BridgeConfig) ([]byte, error) {
	// TODO: @Stefan-Ethernal Temporary workaround just to be able to run cluster in non-bridge mode, until SC is fixed
	rootERC20PredicateAddr := types.StringToAddress("0xDEAD")
	rootERC20Addr := types.ZeroAddress

	if config != nil {
		rootERC20PredicateAddr = config.RootERC20PredicateAddr
		rootERC20Addr = config.RootNativeERC20Addr
	}

	params := &contractsapi.InitializeChildERC20PredicateFunction{
		NewL2StateSender:          contracts.L2StateSenderContract,
		NewStateReceiver:          contracts.StateReceiverContract,
		NewRootERC20Predicate:     rootERC20PredicateAddr,
		NewChildTokenTemplate:     contracts.ChildERC20Contract,
		NewNativeTokenRootAddress: rootERC20Addr,
	}

	return params.EncodeAbi()
}

// getInitNativeERC20Input builds input parameters for NativeERC20 SC initialization
func getInitNativeERC20Input(nativeTokenName, nativeTokenSymbol string, nativeTokenDecimals uint8,
	rootTokenAddr, childPredicateAddr types.Address) ([]byte, error) {
	params := &contractsapi.InitializeNativeERC20Function{
		Name_:      nativeTokenName,
		Symbol_:    nativeTokenSymbol,
		Decimals_:  nativeTokenDecimals,
		RootToken_: rootTokenAddr,
		Predicate_: childPredicateAddr,
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
