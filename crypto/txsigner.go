package crypto

import (
	"fmt"
	"math/big"

	"crypto/ecdsa"

	"github.com/umbracle/minimal/chain"
	rlpv2 "github.com/umbracle/minimal/rlpv2"
	"github.com/umbracle/minimal/types"
)

// TxSigner recovers data from a transaction
type TxSigner interface {
	// Hash returns the hash of the transaction
	Hash(tx *types.Transaction) types.Hash

	// Sender returns the sender to the transaction
	Sender(tx *types.Transaction) (types.Address, error)

	// SignTx signs a transaction
	SignTx(tx *types.Transaction, priv *ecdsa.PrivateKey) (*types.Transaction, error)
}

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

var signerPool rlpv2.ArenaPool

func calcTxHash(tx *types.Transaction, chainID uint64) types.Hash {
	a := signerPool.Get()
	defer signerPool.Put(a)

	v := a.NewArray()
	v.Set(a.NewUint(tx.Nonce))
	v.Set(a.NewBigInt(tx.GasPrice))
	v.Set(a.NewUint(tx.Gas))
	if tx.To == nil {
		v.Set(a.NewNull())
	} else {
		v.Set(a.NewBytes((*tx.To).Bytes()))
	}
	v.Set(a.NewBigInt(tx.Value))
	v.Set(a.NewBytes(tx.Input))

	// EIP155
	if chainID != 0 {
		v.Set(a.NewUint(chainID))
		v.Set(a.NewUint(0))
		v.Set(a.NewUint(0))
	}

	buf := a.HashTo(nil, v)
	return types.BytesToHash(buf)
}

func (f *FrontierSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, 0)
}

func (f *FrontierSigner) Sender(tx *types.Transaction) (types.Address, error) {
	sig, err := encodeSignature(tx.R, tx.S, byte(tx.V.Uint64()-27))
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

func (f *FrontierSigner) SignTx(tx *types.Transaction, priv *ecdsa.PrivateKey) (*types.Transaction, error) {
	tx = tx.Copy()

	h := f.Hash(tx)

	sig, err := Sign(priv, h[:])
	if err != nil {
		return nil, err
	}

	tx.R = new(big.Int).SetBytes(sig[:32])
	tx.S = new(big.Int).SetBytes(sig[32:64])
	tx.V = new(big.Int).SetBytes([]byte{sig[64] + 27})

	return tx, nil
}

func NewEIP155Signer(chainID uint64) *EIP155Signer {
	return &EIP155Signer{chainID: chainID}
}

type EIP155Signer struct {
	chainID uint64
}

func (e *EIP155Signer) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, e.chainID)
}

func (e *EIP155Signer) Sender(tx *types.Transaction) (types.Address, error) {
	protected := true
	if tx.V.BitLen() <= 8 {
		v := tx.V.Uint64()
		protected = v != 27 && v != 28
	}

	if !protected {
		return (&FrontierSigner{}).Sender(tx)
	}

	return types.Address{}, fmt.Errorf("EIP155 signer not implemented yet")
}

func (e *EIP155Signer) SignTx(tx *types.Transaction, priv *ecdsa.PrivateKey) (*types.Transaction, error) {
	return nil, fmt.Errorf("not implemented")
}

func encodeSignature(R, S *big.Int, V byte) ([]byte, error) {
	if !ValidateSignatureValues(V, R, S, false) {
		return nil, fmt.Errorf("invalid signature")
	}

	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, 65)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = V
	return sig, nil
}
