package polybft

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
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
	validatorAddresses := make([]types.Address, len(polyBFTConfig.InitialValidatorSet))
	validatorPubkeys := make([][4]*big.Int, len(polyBFTConfig.InitialValidatorSet))
	validatorStakes := make([]*big.Int, len(polyBFTConfig.InitialValidatorSet))

	for i, validator := range polyBFTConfig.InitialValidatorSet {
		blsKey, err := hex.DecodeString(validator.BlsKey)
		if err != nil {
			return nil, err
		}

		pubKey, err := bls.UnmarshalPublicKey(blsKey)
		if err != nil {
			return nil, err
		}

		pubKeyBig := pubKey.ToBigInt()

		validatorPubkeys[i] = pubKeyBig
		validatorAddresses[i] = validator.Address
		validatorStakes[i] = new(big.Int).Set(validator.Balance)
	}

	registerMessage, err := bls.MarshalMessageToBigInt([]byte(contracts.PolyBFTRegisterMessage))
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"init": map[string]interface{}{
			"epochReward":   new(big.Int).SetUint64(polyBFTConfig.EpochReward),
			"minStake":      big.NewInt(minStake),
			"minDelegation": big.NewInt(minDelegation),
			"epochSize":     new(big.Int).SetUint64(polyBFTConfig.EpochSize),
		},
		"validatorAddresses": validatorAddresses,
		"validatorPubkeys":   validatorPubkeys,
		"validatorStakes":    validatorStakes,
		"newBls":             contracts.BLSContract, // address of the deployed BLS contract
		"newMessage":         registerMessage,
		"governance":         polyBFTConfig.Governance,
	}

	input, err := contractsapi.ChildValidatorSet.Abi.Methods["initialize"].Encode(params)
	if err != nil {
		return nil, err
	}

	return input, nil
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
