package crypto

import (
	"crypto/ecdsa"
	"fmt"
	"math/bits"

	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
)

// TxSigner is a utility interface used to recover data from a transaction
type TxSigner interface {
	// Hash returns the hash of the transaction
	Hash(tx *types.Transaction) types.Hash

	// Sender returns the sender of the transaction
	Sender(tx *types.Transaction) (types.Address, error)

	// SignTx signs a transaction
	SignTx(tx *types.Transaction, priv *ecdsa.PrivateKey) (*types.Transaction, error)
}

// NewSigner creates a new signer object (EIP155 or FrontierSigner)
func NewSigner(forks chain.ForksInTime, chainID uint64) TxSigner {
	var signer TxSigner

	if forks.EIP155 {
		signer = &EIP155Signer{chainID: chainID}
	} else {
		signer = &FrontierSigner{}
	}
	return signer
}

type FrontierSigner struct {
}

var signerPool fastrlp.ArenaPool

// calcTxHash calculates the transaction hash (keccak256 hash of the RLP value)
func calcTxHash(tx *types.Transaction, chainID uint64) types.Hash {
	a := signerPool.Get()

	v := a.NewArray()
	v.Set(a.NewUint(tx.Nonce))
	v.Set(a.NewBigInt(tx.GasPrice))
	v.Set(a.NewUint(tx.Gas))
	if tx.To == nil {
		v.Set(a.NewNull())
	} else {
		v.Set(a.NewCopyBytes((*tx.To).Bytes()))
	}
	v.Set(a.NewBigInt(tx.Value))
	v.Set(a.NewCopyBytes(tx.Input))

	// EIP155
	if chainID != 0 {
		v.Set(a.NewUint(chainID))
		v.Set(a.NewUint(0))
		v.Set(a.NewUint(0))
	}

	hash := keccak.Keccak256Rlp(nil, v)
	signerPool.Put(a)

	return types.BytesToHash(hash)
}

// Hash is a wrapper function for the calcTxHash, with chainID 0
func (f *FrontierSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, 0)
}

// Sender decodes the signature and returns the sender of the transaction
func (f *FrontierSigner) Sender(tx *types.Transaction) (types.Address, error) {
	sig, err := encodeSignature(tx.R, tx.S, tx.V-27)
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

	tx.R = sig[:32]
	tx.S = sig[32:64]
	tx.V = byte(sig[64] + 27)

	return tx, nil
}

// NewEIP155Signer returns a new EIP155Signer object
func NewEIP155Signer(chainID uint64) *EIP155Signer {
	return &EIP155Signer{chainID: chainID}
}

type EIP155Signer struct {
	chainID uint64
}

// Hash is a wrapper function that calls calcTxHash with the EIP155Signer's chainID
func (e *EIP155Signer) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

// Sender returns the transaction sender
func (e *EIP155Signer) Sender(tx *types.Transaction) (types.Address, error) {
	protected := true

	if vv := uint(tx.V); bits.Len(vv) <= 8 {
		protected = vv != 27 && vv != 28
	}

	if !protected {
		return (&FrontierSigner{}).Sender(tx)
	}

	v := tx.V - byte(e.chainID*2)
	v -= 8
	v -= 27

	sig, err := encodeSignature(tx.R, tx.S, v)
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

	tx.R = sig[:32]
	tx.S = sig[32:64]
	tx.V = byte(sig[64]+35) + (byte(e.chainID) * 2)

	return tx, nil
}

// encodeSignature generates a signature value based on the R, S and V value
func encodeSignature(R, S []byte, V byte) ([]byte, error) {
	if !ValidateSignatureValues(V, R, S) {
		return nil, fmt.Errorf("invalid txn signature")
	}

	sig := make([]byte, 65)
	copy(sig[32-len(R):32], R)
	copy(sig[64-len(S):64], S)
	sig[64] = V

	return sig, nil
}
