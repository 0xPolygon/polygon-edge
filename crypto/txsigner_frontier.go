package crypto

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/umbracle/fastrlp"

	"github.com/0xPolygon/polygon-edge/types"
)

var signerPool fastrlp.ArenaPool

// FrontierSigner implements tx signer interface
type FrontierSigner struct {
	isHomestead bool
}

// NewFrontierSigner is the constructor of FrontierSigner
func NewFrontierSigner(isHomestead bool) *FrontierSigner {
	return &FrontierSigner{
		isHomestead: isHomestead,
	}
}

// Hash is a wrapper function for the calcTxHash, with chainID 0
func (f *FrontierSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, 0)
}

// Sender decodes the signature and returns the sender of the transaction
func (f *FrontierSigner) Sender(tx *types.Transaction) (types.Address, error) {
	refV := big.NewInt(0)
	if tx.V != nil {
		refV.SetBytes(tx.V.Bytes())
	}

	refV.Sub(refV, big27)

	sig, err := encodeSignature(tx.R, tx.S, refV, f.isHomestead)
	if err != nil {
		return types.Address{}, err
	}

	pub, err := Ecrecover(f.Hash(tx).Bytes(), sig)
	if err != nil {
		return types.Address{}, err
	}

	buf := Keccak256(pub[1:])[12:]

	return types.BytesToAddress(buf), nil
}

// SignTx signs the transaction using the passed in private key
func (f *FrontierSigner) SignTx(
	tx *types.Transaction,
	privateKey *ecdsa.PrivateKey,
) (*types.Transaction, error) {
	tx = tx.Copy()

	h := f.Hash(tx)

	sig, err := Sign(privateKey, h[:])
	if err != nil {
		return nil, err
	}

	tx.R = new(big.Int).SetBytes(sig[:32])
	tx.S = new(big.Int).SetBytes(sig[32:64])
	tx.V = new(big.Int).SetBytes(f.calculateV(sig[64]))

	return tx, nil
}

// calculateV returns the V value for transactions pre EIP155
func (f *FrontierSigner) calculateV(parity byte) []byte {
	reference := big.NewInt(int64(parity))
	reference.Add(reference, big27)

	return reference.Bytes()
}
