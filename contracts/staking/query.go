package staking

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/contracts/abis"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/abi"
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

	web3Addresses, ok := results["0"].([]web3.Address)

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

type StoreInterface interface {
}

type BlockChainStoreQueryHandler interface {
	// Header returns the current header of the chain (genesis if empty)
	Header() *types.Header
}

func QueryValidators(t TxQueryHandler, from types.Address, store BlockChainStoreQueryHandler) ([]types.Address, error) {
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

	addrs, err := DecodeValidators(method, res.ReturnValue)
	if err != nil {
		return nil, err
	}

	if len(addrs) == 0 {
		fmt.Println("addrs is empty: ", len(addrs))
		return []types.Address{}, nil
	}

	u := NewUpHash(len(addrs))
	headHash := store.Header().Hash //取链上最新区块的hash
	fmt.Println(" head hash ", headHash)
	factor := int64(binary.BigEndian.Uint64(headHash.Bytes()))
	resultSeqs, err := u.GenHash(factor)
	if err != nil {
		return nil, err
	}

	realAddr := make([]types.Address, len(addrs))
	for _, v := range resultSeqs {
		realAddr = append(realAddr, addrs[v])
	}

	return realAddr, nil
}
