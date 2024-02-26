package crypto

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// Magic numbers, taken from the Ethereum, used in the calculation of the V value
// Only matters in pre-EIP-2930 (pre-Berlin) transactions
var (
	big27 = big.NewInt(27) // pre-EIP-155
	big35 = big.NewInt(35) // EIP-155
)

// RLP encoding helper
var arenaPool fastrlp.ArenaPool

// TxSigner is a utility interface used to work with transaction signatures
type TxSigner interface {
	// Hash returns the hash of the transaction
	Hash(*types.Transaction) types.Hash

	// Sender returns the sender of the transaction
	Sender(*types.Transaction) (types.Address, error)

	// SingTx takes the original transaction as input and returns its signed version
	SignTx(*types.Transaction, *ecdsa.PrivateKey) (*types.Transaction, error)
}

// NewSigner creates a new signer based on currently supported forks
func NewSigner(forks chain.ForksInTime, chainID uint64) TxSigner {
	if forks.London {
		return NewLondonSigner(chainID)
	}

	if forks.Berlin {
		return NewBerlinSigner(chainID)
	}

	if forks.EIP155 {
		return NewEIP155Signer(chainID)
	}

	if forks.Homestead {
		return NewHomesteadSigner()
	}

	return NewFrontierSigner()
}

// encodeSignature generates a signature based on the R, S and parity values
//
// The signature encoding format is as follows:
// (32-bytes R, 32-bytes S, 1-byte parity)
//
// Note: although the signature value V, based on different standards, is calculated and encoded in different ways,
// the encodeSignature function expects parity of Y coordinate as third input and that is what will be encoded
func encodeSignature(r, s, parity *big.Int, isHomestead bool) ([]byte, error) {
	if !ValidateSignatureValues(parity, r, s, isHomestead) {
		return nil, errors.New("signature encoding failed, because transaction signature is invalid")
	}

	signature := make([]byte, 65)

	copy(signature[32-len(r.Bytes()):32], r.Bytes())
	copy(signature[64-len(s.Bytes()):64], s.Bytes())
	signature[64] = byte(parity.Int64())

	return signature, nil
}

// recoverAddress recovers the sender address from the transaction hash and signature R, S and parity values
func recoverAddress(txHash types.Hash, r, s, parity *big.Int, isHomestead bool) (types.Address, error) {
	signature, err := encodeSignature(r, s, parity, isHomestead)
	if err != nil {
		return types.ZeroAddress, err
	}

	publicKey, err := Ecrecover(txHash.Bytes(), signature)
	if err != nil {
		return types.ZeroAddress, err
	}

	if len(publicKey) == 0 || publicKey[0] != 4 {
		return types.ZeroAddress, errors.New("invalid public key")
	}

	// First byte of the publicKey indicates that it is serialized in uncompressed form
	// (it has the value 0x04), so we ommit that
	hash := Keccak256(publicKey[1:])

	address := hash[12:]

	return types.BytesToAddress(address), nil
}

// validateTxChainID checks if the transaction chain ID matches the expected chain ID
func validateTxChainID(tx *types.Transaction, chainID uint64) error {
	txChainID := tx.ChainID()

	if txChainID == nil || txChainID.Uint64() != chainID {
		return fmt.Errorf("%w: have %d want %d", errInvalidChainID, tx.ChainID(), chainID)
	}

	return nil
}
