package e2e

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
)

// TODO: @Stefan-Ethernal use SystemState here?
func getValidators(txRelayer txrelayer.TxRelayer, senderAddr ethgo.Address) (polybft.AccountSet, error) {
	function := polybft.StateFunctionsABI.GetMethod("getCurrentValidatorSet")

	input, err := function.Encode([]interface{}{})
	if err != nil {
		return nil, err
	}

	response, err := txRelayer.Call(senderAddr, ethgo.Address(contracts.ValidatorSetContract), input)
	if err != nil {
		return nil, err
	}

	byteResponse, err := hex.DecodeHex(response)
	if err != nil {
		return nil, fmt.Errorf("unable to decode hex response, %w", err)
	}

	decodedOutput, err := function.Outputs.Decode(byteResponse)
	if err != nil {
		return nil, err
	}

	decodedMap, ok := decodedOutput.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed to decode output to map")
	}

	addrs, ok := decodedMap["0"].([]ethgo.Address)
	if !ok {
		return nil, fmt.Errorf("failed to decode addresses of the current validator set")
	}

	function = polybft.StateFunctionsABI.GetMethod("getValidator")

	accountSet := polybft.AccountSet{}

	for _, addr := range addrs {
		input, err = function.Encode([]interface{}{addr})
		if err != nil {
			return nil, err
		}

		response, err := txRelayer.Call(senderAddr, ethgo.Address(contracts.ValidatorSetContract), input)
		if err != nil {
			return nil, err
		}

		byteResponse, err := hex.DecodeHex(response)
		if err != nil {
			return nil, fmt.Errorf("unable to decode hex response, %w", err)
		}

		decodedOutput, err := function.Outputs.Decode(byteResponse)
		if err != nil {
			return nil, err
		}

		decodedMap, ok := decodedOutput.(map[string]interface{})
		if !ok {
			return nil, errors.New("failed to decode output to map")
		}

		decodedMap, ok = decodedMap["0"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("failed to decode validator data")
		}

		pubKey, err := bls.UnmarshalPublicKeyFromBigInt(decodedMap["blsKey"].([4]*big.Int))
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal BLS public key: %w", err)
		}

		totalStake, ok := decodedMap["totalStake"].(*big.Int)
		if !ok {
			return nil, fmt.Errorf("failed to decode total stake")
		}

		val := &polybft.ValidatorMetadata{
			Address:     types.Address(addr),
			BlsKey:      pubKey,
			VotingPower: new(big.Int).Set(totalStake),
		}
		accountSet = append(accountSet, val)
	}

	sort.Slice(accountSet, func(i, j int) bool {
		return bytes.Compare(accountSet[i].Address[:], accountSet[j].Address[:]) < 0
	})

	return accountSet, nil
}
