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

	v, r, s := tx.RawSignatureValues()
	if v != nil {
		refV.SetBytes(v.Bytes())
	}

	refV.Sub(refV, big27)

	return recoverAddress(f.Hash(tx), r, s, refV, f.isHomestead)
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

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])
	v := new(big.Int).SetBytes(f.calculateV(sig[64]))

	tx.SetSignatureValues(v, r, s)

	return tx, nil
}

// calculateV returns the V value for transactions pre EIP155
func (f *FrontierSigner) calculateV(parity byte) []byte {
	reference := big.NewInt(int64(parity))
	reference.Add(reference, big27)

	return reference.Bytes()
}
