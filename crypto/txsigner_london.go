package crypto

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// LondonSigner implements signer for EIP-1559
type LondonSigner struct {
	chainID        uint64
	isHomestead    bool
	fallbackSigner TxSigner
}

// NewLondonSigner returns a new LondonSigner object
func NewLondonSigner(chainID uint64, isHomestead bool, fallbackSigner TxSigner) *LondonSigner {
	return &LondonSigner{
		chainID:        chainID,
		isHomestead:    isHomestead,
		fallbackSigner: fallbackSigner,
	}
}

// Hash is a wrapper function that calls calcTxHash with the LondonSigner's fields
func (e *LondonSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

// Sender returns the transaction sender
func (e *LondonSigner) Sender(tx *types.Transaction) (types.Address, error) {
	// Apply fallback signer for non-dynamic-fee-txs
	if tx.Type != types.DynamicFeeTx {
		return e.fallbackSigner.Sender(tx)
	}

	sig, err := encodeSignature(tx.R, tx.S, tx.V, e.isHomestead)
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
func (e *LondonSigner) SignTx(tx *types.Transaction, pk *ecdsa.PrivateKey) (*types.Transaction, error) {
	// Apply fallback signer for non-dynamic-fee-txs
	if tx.Type != types.DynamicFeeTx {
		return e.fallbackSigner.SignTx(tx, pk)
	}

	tx = tx.Copy()

	h := e.Hash(tx)

	sig, err := Sign(pk, h[:])
	if err != nil {
		return nil, err
	}

	tx.R = new(big.Int).SetBytes(sig[:32])
	tx.S = new(big.Int).SetBytes(sig[32:64])
	tx.V = new(big.Int).SetBytes(e.calculateV(sig[64]))

	return tx, nil
}

// calculateV returns the V value for transaction signatures. Based on EIP155
func (e *LondonSigner) calculateV(parity byte) []byte {
	return big.NewInt(int64(parity)).Bytes()
}
