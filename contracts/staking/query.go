package staking

import (
	"math/big"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/state"
	"github.com/0xPolygon/polygon-sdk/types"
)

const (
	ValidatorsFuncInterface = "validators()"
)

var (
	// staking contract address
	AddrStakingContract = types.StringToAddress("1001")
)

func getFunctionSelector(funcInterface string) [4]byte {
	// First 4bytes of Keccak256(func interface)
	// e.g. Keccak256("validators()")[:4]
	var data [4]byte
	res := crypto.Keccak256([]byte(funcInterface))
	copy(data[:], res[:4])
	return data
}

func parseValidators(rawData []byte) []types.Address {
	// ReturnData is like this (separate per 32bytes)
	// Currently we're expecting this data has only array of addresses
	//
	//  0 byte | 0x00000....20 (offset of the beginning of array => 0x20 = 32 bytes)
	// 32 byte | 0x00000....04 (number of elements in array => 4)
	// 64 byte | 0x1a85F....b6 (address 1)
	// 96 byte | 0xxxxxx....xx (address 2)
	// ...
	// x  byte | 0x........... (address n)
	// ...

	offset := new(big.Int).SetBytes(rawData[0:32]).Uint64()
	num := new(big.Int).SetBytes(rawData[offset : offset+32]).Uint64()

	validators := make([]types.Address, num)
	for i := uint64(0); i < num; i++ {
		begin := offset + (i+1)*32
		validators[i] = types.BytesToAddress(rawData[begin : begin+32])
	}
	return validators
}

func QueryValidators(t *state.Transition, from types.Address) ([]types.Address, error) {
	selector := getFunctionSelector(ValidatorsFuncInterface)
	res, err := t.Apply(&types.Transaction{
		From:     from,
		To:       &AddrStakingContract,
		Value:    big.NewInt(0),
		Input:    selector[:],
		GasPrice: big.NewInt(0),
		Gas:      100000000,
		Nonce:    t.GetNonce(from),
	})
	if err != nil {
		return nil, err
	}
	if res.Failed() {
		return nil, res.Err
	}

	return parseValidators(res.ReturnValue), nil
}
