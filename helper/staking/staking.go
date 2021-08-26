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

// getAddressMapping returns the key for the SC storage mapping (address => something)
//
// More information:
// https://docs.soliditylang.org/en/latest/internals/layout_in_storage.html
func getAddressMapping(address types.Address, slot int64) []byte {
	bigSlot := big.NewInt(slot)

	finalSlice := append(
		padLeftOrTrim(address.Bytes(), 32),
		padLeftOrTrim(bigSlot.Bytes(), 32)...,
	)
	keccakValue := keccak.Keccak256(nil, finalSlice)

	return keccakValue
}

// getIndexWithOffset is a helper method for adding an offset to the already found keccak hash
func getIndexWithOffset(keccakHash []byte, offset int64) []byte {
	bigOffset := big.NewInt(offset)
	bigKeccak := big.NewInt(0).SetBytes(keccakHash) // TODO check if this is correct

	return (big.NewInt(0).Add(bigKeccak, bigOffset)).Bytes()
}

// GetStorageIndexes is a helper function for getting the correct indexes
// of the storage slots which need to be modified during bootstrap.
//
// It is SC dependant, and based on the SC located at:
// https://github.com/0xPolygon/staking-contracts/
func GetStorageIndexes(address types.Address, index int64) *StorageIndexes {
	storageIndexes := StorageIndexes{}

	// Get the indexes for the mappings
	storageIndexes.AddressToIsValidatorIndex = getAddressMapping(address, addressToIsValidatorSlot)
	storageIndexes.AddressToStakedAmountIndex = getAddressMapping(address, addressToStakedAmountSlot)
	storageIndexes.AddressToValidatorIndexIndex = getAddressMapping(address, addressToValidatorIndexSlot)

	// Get the indexes for _validators, _stakedAmount
	paddedStakedAmount := padLeftOrTrim(big.NewInt(stakedAmountSlot).Bytes(), 32)
	paddedValidatorsArray := padLeftOrTrim(big.NewInt(validatorsSlot).Bytes(), 32)

	// Index for regular types is calculated as the keccak256(slot)
	storageIndexes.StakedAmountIndex = keccak.Keccak256(nil, paddedStakedAmount)

	// Index for array types is calculated as keccak(slot) + index
	storageIndexes.ValidatorsIndex = getIndexWithOffset(keccak.Keccak256(nil, paddedValidatorsArray), index)

	return &storageIndexes
}

// StorageIndexes is a wrapper for different storage indexes that
// need to be modified
type StorageIndexes struct {
	ValidatorsIndex              []byte // []address
	AddressToIsValidatorIndex    []byte // mapping(address => bool)
	AddressToStakedAmountIndex   []byte // mapping(address => uint256)
	AddressToValidatorIndexIndex []byte // mapping(address => uint256)
	StakedAmountIndex            []byte // uint256
}

// Slot definitions for SC storage
var (
	validatorsSlot              = int64(1) // Slot 1
	addressToIsValidatorSlot    = int64(2) // Slot 2
	addressToStakedAmountSlot   = int64(3) // Slot 3
	addressToValidatorIndexSlot = int64(4) // Slot 4
	stakedAmountSlot            = int64(5) // Slot 5
)
