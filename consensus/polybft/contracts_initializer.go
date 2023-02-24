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
	// TODO: @Stefan-Ethernal Temporal workaround just to be able to run cluster in non-bridge mode, until SC is fixed
	rootERC20PredicateAddr := types.StringToAddress("0xDEAD")
	// TODO: @Stefan-Ethernal Provide rootToken address?
	// rootERC20Addr := types.ZeroAddress
	// rootERC20Addr = config.RootERC20Addr

	if config != nil {
		rootERC20PredicateAddr = config.RootERC20PredicateAddr
	}

	params := &contractsapi.InitializeChildERC20PredicateFunction{
		NewL2StateSender:      contracts.L2StateSenderContract,
		NewStateReceiver:      contracts.StateReceiverContract,
		NewRootERC20Predicate: rootERC20PredicateAddr,
		NewChildTokenTemplate: contracts.ChildERC20Contract,
		// TODO: Is this correct? If provided config.RootERC20Addr,
		// rootToken will be mapped to native token on child chain, but not on root chain
		// (and consequently RootERC20Predicate would try to map token and
		// invoke mapping on child chain, though rootToken is already mapped, which causes an error)
		NewNativeTokenRootAddress: types.ZeroAddress,
		NewNativeTokenName:        nativeTokenName,
		NewNativeTokenSymbol:      nativeTokenSymbol,
		NewNativeTokenDecimals:    nativeTokenDecimals,
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
