package staking

import (
	"math/big"

	"github.com/0xPolygon/polygon-sdk/helper/keccak"
	"github.com/0xPolygon/polygon-sdk/types"
)

// padLeftOrTrim left-pads the passed in byte array to the specified size,
// or trims the array if it exceeds the passed in size
func padLeftOrTrim(bb []byte, size int) []byte {
	l := len(bb)
	if l == size {
		return bb
	}
	if l > size {
		return bb[l-size:]
	}
	tmp := make([]byte, size)
	copy(tmp[size-l:], bb)

	return tmp
}

// GetStorageMappingIndex is a helper function for getting the index
// of the storage slot to which a prestaked balance should be written
// It is SC dependant, and based on the SC located at:
// https://github.com/0xPolygon/staking-contracts/
func GetStorageMappingIndex(address types.Address, slot int64) []byte {
	bigSlot := big.NewInt(slot)

	finalSlice := append(
		padLeftOrTrim(address.Bytes(), 32),
		padLeftOrTrim(bigSlot.Bytes(), 32)...,
	)
	keccakValue := keccak.Keccak256(nil, finalSlice)

	return keccakValue
}
