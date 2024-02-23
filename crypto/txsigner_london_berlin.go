package crypto

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// LondonOrBerlinSigner implements signer for london and berlin hard forks
type LondonOrBerlinSigner struct {
	chainID        uint64
	isHomestead    bool
	fallbackSigner TxSigner
}

// NewLondonOrBerlinSigner returns new LondonOrBerlinSigner object that accepts
// - EIP-1559 dynamic fee transactions
// - EIP-2930 access list transactions,
// - EIP-155 replay protected transactions, and
// - legacy Homestead transactions.
func NewLondonOrBerlinSigner(chainID uint64, isHomestead bool, fallbackSigner TxSigner) *LondonOrBerlinSigner {
	return &LondonOrBerlinSigner{
		chainID:        chainID,
		isHomestead:    isHomestead,
		fallbackSigner: fallbackSigner,
	}
}

// Hash is a wrapper function that calls calcTxHash with the LondonSigner's fields
func (e *LondonOrBerlinSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

// Sender returns the transaction sender
func (e *LondonOrBerlinSigner) Sender(tx *types.Transaction) (types.Address, error) {
	if tx.Type() != types.DynamicFeeTxType && tx.Type() != types.AccessListTxType {
		return e.fallbackSigner.Sender(tx)
	}

	v, r, s := tx.RawSignatureValues()

	return recoverAddress(e.Hash(tx), r, s, v, e.isHomestead)
}

// SignTx signs the transaction using the passed in private key
func (e *LondonOrBerlinSigner) SignTx(tx *types.Transaction, pk *ecdsa.PrivateKey) (*types.Transaction, error) {
	if tx.Type() != types.DynamicFeeTxType && tx.Type() != types.AccessListTxType {
		return e.fallbackSigner.SignTx(tx, pk)
	}

	tx = tx.Copy()

	h := e.Hash(tx)

	sig, err := Sign(pk, h[:])
	if err != nil {
		return nil, err
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetBytes(e.calculateV(sig[64]))
	tx.SetSignatureValues(v, r, s)

	return tx, nil
}

// calculateV returns the V value for transaction signatures. Based on EIP155
func (e *LondonOrBerlinSigner) calculateV(parity byte) []byte {
	return big.NewInt(int64(parity)).Bytes()
}
