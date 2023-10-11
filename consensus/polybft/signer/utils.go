package bls

import (
	"bytes"
	"crypto/rand"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo/abi"
)

var (
	addressABIType = abi.MustNewType("address")
	uint256ABIType = abi.MustNewType("uint256")
)

// GenerateBlsKey creates a random private and its corresponding public keys
func GenerateBlsKey() (*PrivateKey, error) {
	s, err := randomK(rand.Reader)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{s: s}, nil
}

// CreateRandomBlsKeys creates an array of random private and their corresponding public keys
func CreateRandomBlsKeys(total int) ([]*PrivateKey, error) {
	blsKeys := make([]*PrivateKey, total)

	for i := 0; i < total; i++ {
		blsKey, err := GenerateBlsKey()
		if err != nil {
			return nil, err
		}

		blsKeys[i] = blsKey
	}

	return blsKeys, nil
}

// MakeKOSKSignature creates KOSK signature which prevents rogue attack
func MakeKOSKSignature(privateKey *PrivateKey, address types.Address,
	chainID int64, domain []byte, supernetManagerAddr types.Address) (*Signature, error) {
	spenderABI, err := addressABIType.Encode(address)
	if err != nil {
		return nil, err
	}

	supernetManagerABI, err := addressABIType.Encode(supernetManagerAddr)
	if err != nil {
		return nil, err
	}

	chainIDABI, err := uint256ABIType.Encode(big.NewInt(chainID))
	if err != nil {
		return nil, err
	}

	// ethgo pads address to 32 bytes, but solidity doesn't (keeps it 20 bytes)
	// that's why we are skipping first 12 bytes
	message := bytes.Join([][]byte{spenderABI[12:], supernetManagerABI[12:], chainIDABI}, nil)

	return privateKey.Sign(message, domain)
}
