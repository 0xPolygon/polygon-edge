package staking

import (
	"errors"
	"math/big"

	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/contracts/abis"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

var (
	// staking contract address
	AddrStakingContract = types.StringToAddress("1001")

	// Gas limit used when querying the validator set
	queryGasLimit uint64 = 100000
)

func DecodeValidators(method *abi.Method, returnValue []byte) ([]types.Address, error) {
	decodedResults, err := method.Outputs.Decode(returnValue)
	if err != nil {
		return nil, err
	}

	results, ok := decodedResults.(map[string]interface{})
	if !ok {
		return nil, errors.New("failed type assertion from decodedResults to map")
	}

	web3Addresses, ok := results["0"].([]ethgo.Address)

	if !ok {
		return nil, errors.New("failed type assertion from results[0] to []ethgo.Address")
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

func QueryValidators(t TxQueryHandler, contract types.Address, from types.Address) ([]types.Address, error) {
	method, ok := abis.StakingABI.Methods["validators"]
	if !ok {
		return nil, errors.New("validators method doesn't exist in Staking contract ABI")
	}

	stakingContract := AddrStakingContract
	if contract != types.ZeroAddress {
		stakingContract = contract
	}

	selector := method.ID()
	res, err := t.Apply(&types.Transaction{
		From:     from,
		To:       &stakingContract,
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

func QueryBlockRewardsPayment(t TxQueryHandler, contract types.Address, from types.Address) (string, error) {
	method, ok := abis.CustomABI.Methods["getBlockReward"]
	if !ok {
		return "0", errors.New("getBlockReward method doesn't exist in Staking contract ABI")
	}

	selector := method.ID()
	res, err := t.Apply(&types.Transaction{
		From:     from,
		To:       &contract,
		Value:    big.NewInt(0),
		Input:    selector,
		GasPrice: big.NewInt(0),
		Gas:      queryGasLimit,
		Nonce:    t.GetNonce(from),
	})

	if err != nil {
		return "0", err
	}

	if res.Failed() {
		return "0", res.Err
	}

	result, err := method.Outputs.Decode(res.ReturnValue)
	if err != nil {
		return "0", err
	}

	outputMap, ok := result.(map[string]interface{})
	if !ok {
		return "0", errors.New("failed type assertion from getBlockReward returnValue to map")
	}

	blockReward, ok := outputMap["0"].(*big.Int)
	if !ok {
		return "0", errors.New("failed type assertion from outputMap[0] to big.Int")
	}

	return blockReward.String(), nil
}
