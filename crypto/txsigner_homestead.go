package crypto

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// HomesteadSigner may be used for signing pre-EIP155 transactions
type HomesteadSigner struct {
	*FrontierSigner
}

// NewHomesteadSigner returns new FrontierSigner object (constructor)
//
// HomesteadSigner accepts the following types of transactions:
//   - pre-EIP-155 transactions
func NewHomesteadSigner() *HomesteadSigner {
	return &HomesteadSigner{
		FrontierSigner: NewFrontierSigner(),
	}
}

// Hash returns the keccak256 hash of the transaction
//
// The pre-EIP-155 transaction hash preimage is as follows:
// RLP(nonce, gasPrice, gas, to, value, input)
//
// Specification: https://eips.ethereum.org/EIPS/eip-155#specification
//
// Note: Since the hash is calculated in the same way as with FrontierSigner, this is just a wrapper method
func (signer *HomesteadSigner) Hash(tx *types.Transaction) types.Hash {
	return signer.FrontierSigner.Hash(tx)
}

// Sender returns the sender of the transaction
func (signer *HomesteadSigner) Sender(tx *types.Transaction) (types.Address, error) {
	return signer.sender(tx, true)
}

// SingTx takes the original transaction as input and returns its signed version
func (signer *HomesteadSigner) SignTx(tx *types.Transaction, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	return signer.signTx(tx, privateKey, func(v, r, s *big.Int) error {
		// Homestead hard-fork introduced the rule that the S value
		// must be inclusively lower than the half of the secp256k1 curve order
		// Specification: https://eips.ethereum.org/EIPS/eip-2#specification (2)
		if s.Cmp(secp256k1NHalf) > 0 {
			return errors.New("SignTx method: S must be inclusively lower than secp256k1n/2")
		}

		return nil
	})
}
