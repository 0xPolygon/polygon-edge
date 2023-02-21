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
	nativeTokenName   = "Polygon"
	nativeTokenSymbol = "MATIC"
)

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

		return result.Err
	}

	return nil
}
