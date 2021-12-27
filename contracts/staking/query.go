package staking

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-sdk/contracts/abis"
	"github.com/0xPolygon/polygon-sdk/state/runtime"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
)

var (
	// staking contract address
	AddrStakingContract = types.StringToAddress("1001")

	// Gas limit used when querying the validator set
	queryGasLimit uint64 = 100000
)

func extractResponse(method *abi.Method, returnValue []byte) (map[string]interface{}, error) {
	decodedResults, err := method.Outputs.Decode(returnValue)
	if err != nil {
		return nil, err
	}

	results, ok := decodedResults.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed type assertion from decodedResults to map")
	}

	return results, nil
}

func DecodeValidators(method *abi.Method, returnValue []byte) ([]types.Address, error) {
	extractedMap, extractErr := extractResponse(method, returnValue)
	if extractErr != nil {
		return nil, fmt.Errorf("unable to extract results, %v", extractErr)
	}

	web3Addresses, ok := extractedMap["0"].([]web3.Address)
	if !ok {
		return nil, errors.New("failed type assertion from results[0] to []web3.Address")
	}

	addresses := make([]types.Address, len(web3Addresses))
	for idx, waddr := range web3Addresses {
		addresses[idx] = types.Address(waddr)
	}

	return addresses, nil
}

type TxQueryHandler interface {
	Apply(*types.Transaction) (*runtime.ExecutionResult, error)
	GetNonce(types.Address) uint64
}

func QueryValidators(t TxQueryHandler, from types.Address) ([]types.Address, error) {
	method, ok := abis.StakingABI.Methods["validators"]
	if !ok {
		return nil, errors.New("validators method doesn't exist in Staking contract ABI")
	}

	selector := method.ID()
	res, err := t.Apply(&types.Transaction{
		From:     from,
		To:       &AddrStakingContract,
		Value:    big.NewInt(0),
		Input:    selector,
		GasPrice: big.NewInt(0),
		Gas:      queryGasLimit,
		Nonce:    t.GetNonce(from),
	})
	if err != nil {
		return nil, err
	}
	if res.Failed() {
		return nil, res.Err
	}

	return DecodeValidators(method, res.ReturnValue)
}

func QueryStakedAmount(t TxQueryHandler, from types.Address) (*big.Int, error) {
	method, ok := abis.StakingABI.Methods["stakedAmount"]
	if !ok {
		return nil, errors.New("stakedAmount method doesn't exist in Staking contract ABI")
	}

	selector := method.ID()
	res, err := t.Apply(&types.Transaction{
		From:     types.ZeroAddress,
		To:       &AddrStakingContract,
		Value:    big.NewInt(0),
		Input:    selector,
		GasPrice: big.NewInt(0),
		Gas:      queryGasLimit,
		Nonce:    t.GetNonce(from),
	})
	if err != nil {
		return nil, err
	}
	if res.Failed() {
		return nil, res.Err
	}

	extractedMap, extractErr := extractResponse(method, res.ReturnValue)
	if extractErr != nil {
		return nil, fmt.Errorf("unable to extract results, %v", extractErr)
	}

	validatorStake, ok := extractedMap["0"].(*big.Int)
	if !ok {
		return nil, errors.New("failed type assertion from results[0] to *big.Int")
	}

	return validatorStake, nil
}
