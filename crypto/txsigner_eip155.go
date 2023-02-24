package crypto

import (
	"crypto/ecdsa"
	"math/big"
	"math/bits"

	"github.com/0xPolygon/polygon-edge/types"
)

type EIP155Signer struct {
	chainID     uint64
	isHomestead bool
}

// NewEIP155Signer returns a new EIP155Signer object
func NewEIP155Signer(chainID uint64, isHomestead bool) *EIP155Signer {
	return &EIP155Signer{
		chainID:     chainID,
		isHomestead: isHomestead,
	}
}

// Hash is a wrapper function that calls calcTxHash with the EIP155Signer's chainID
func (e *EIP155Signer) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

// Sender returns the transaction sender
func (e *EIP155Signer) Sender(tx *types.Transaction) (types.Address, error) {
	protected := true

	// Check if v value conforms to an earlier standard (before EIP155)
	bigV := big.NewInt(0)
	if tx.V != nil {
		bigV.SetBytes(tx.V.Bytes())
	}

	if vv := bigV.Uint64(); bits.Len(uint(vv)) <= 8 {
		protected = vv != 27 && vv != 28
	}

	if !protected {
		return (&FrontierSigner{}).Sender(tx)
	}

	// Reverse the V calculation to find the original V in the range [0, 1]
	// v = CHAIN_ID * 2 + 35 + {0, 1}
	mulOperand := big.NewInt(0).Mul(big.NewInt(int64(e.chainID)), big.NewInt(2))
	bigV.Sub(bigV, mulOperand)
	bigV.Sub(bigV, big35)

	sig, err := encodeSignature(tx.R, tx.S, bigV, e.isHomestead)
	if err != nil {
		return types.Address{}, err
	}

	pub, err := Ecrecover(e.Hash(tx).Bytes(), sig)
	if err != nil {
		return types.Address{}, err
	}

	buf := Keccak256(pub[1:])[12:]

	return types.BytesToAddress(buf), nil
}

// SignTx signs the transaction using the passed in private key
func (e *EIP155Signer) SignTx(
	tx *types.Transaction,
	privateKey *ecdsa.PrivateKey,
) (*types.Transaction, error) {
	tx = tx.Copy()

	h := e.Hash(tx)

	sig, err := Sign(privateKey, h[:])
	if err != nil {
		return nil, err
	}

	tx.R = new(big.Int).SetBytes(sig[:32])
	tx.S = new(big.Int).SetBytes(sig[32:64])
	tx.V = new(big.Int).SetBytes(e.calculateV(sig[64]))

	return tx, nil
}

// calculateV returns the V value for transaction signatures. Based on EIP155
func (e *EIP155Signer) calculateV(parity byte) []byte {
	reference := big.NewInt(int64(parity))
	reference.Add(reference, big35)

	mulOperand := big.NewInt(0).Mul(big.NewInt(int64(e.chainID)), big.NewInt(2))

	reference.Add(reference, mulOperand)

	return reference.Bytes()
}
