package ibft

import (
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/btcsuite/btcd/btcec"
	"github.com/ethereum/go-ethereum/rlp"
)

// S256 is the secp256k1 elliptic curve
var S256 = btcec.S256()

func RLPHash(v interface{}) (h types.Hash) {
	hw := keccak.NewKeccak256()
	rlp.Encode(hw, v)
	hw.Sum(h[:0])
	return h
}

// GetSignatureAddress gets the signer address from the signature
func GetSignatureAddress(data []byte, sig []byte) (types.Address, error) {
	// 1. Keccak data
	hashData := crypto.Keccak256(data)
	// 2. Recover public key
	pubkey, err := crypto.SigToPub(hashData, sig)
	if err != nil {
		return types.Address{}, err
	}
	return crypto.PubKeyToAddress(pubkey), nil
}

func CheckValidatorSignature(valSet ValidatorSet, data []byte, sig []byte) (types.Address, error) {
	// 1. Get signature address
	signer, err := GetSignatureAddress(data, sig)
	if err != nil {
		return types.Address{}, err
	}

	// 2. Check validator
	if _, val := valSet.GetByAddress(signer); val != nil {
		return val.Address(), nil
	}

	return types.Address{}, ErrUnauthorizedAddress
}
