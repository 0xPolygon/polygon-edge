package crypto

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// LondonSigner implements signer for EIP-1559
type Eip2930Signer struct {
	chainID        uint64
	isHomestead    bool
	fallbackSigner TxSigner
}

// NewLondonSigner returns a new LondonSigner object
func NewEip2930Signer(chainID uint64, isHomestead bool, fallbackSigner TxSigner) *Eip2930Signer {
	return &Eip2930Signer{
		chainID:        chainID,
		isHomestead:    isHomestead,
		fallbackSigner: fallbackSigner,
	}
}

// Hash is a wrapper function that calls calcTxHash with the LondonSigner's fields
func (e *Eip2930Signer) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

// Sender returns the transaction sender
func (e *Eip2930Signer) Sender(tx *types.Transaction) (types.Address, error) {
	// Apply fallback signer for non-accessList-txs
	if tx.Type() != types.AccessListTx {
		return e.fallbackSigner.Sender(tx)
	}

	v, r, s := tx.RawSignatureValues()
	sig, err := encodeSignature(r, s, v, e.isHomestead)
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
func (e *Eip2930Signer) SignTx(tx *types.Transaction, pk *ecdsa.PrivateKey) (*types.Transaction, error) {
	// Apply fallback signer for non-accessList-txs
	if tx.Type() != types.AccessListTx {
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
func (e *Eip2930Signer) calculateV(parity byte) []byte {
	return big.NewInt(int64(parity)).Bytes()
}
