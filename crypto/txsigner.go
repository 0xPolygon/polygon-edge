package crypto

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"math/bits"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

var (
	ErrUnsupportedTxSignatures = errors.New("tx doesn't support signature values")
)

// TxSigner is a utility interface used to recover data from a transaction
type TxSigner interface {
	// Hash returns the hash of the transaction
	Hash(tx *types.Transaction) types.Hash

	// Sender returns the sender of the transaction
	Sender(tx *types.Transaction) (types.Address, error)

	// SignTx signs a transaction
	SignTx(tx *types.Transaction, priv *ecdsa.PrivateKey) (*types.Transaction, error)

	// CalculateV calculates the V value based on the type of signer used
	CalculateV(parity byte) []byte
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

func marshalLegacyTxRLPForDigest(
	a *fastrlp.Arena,
	tx *types.LegacyTransaction,
	chainID uint64,
) *fastrlp.Value {
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

	return v
}

func marshalStateTxRLPForDigest(
	a *fastrlp.Arena,
	tx *types.StateTransaction,
	chainID uint64,
) *fastrlp.Value {
	v := a.NewArray()

	v.Set(a.NewUint(tx.Nonce))

	if tx.To == nil {
		v.Set(a.NewNull())
	} else {
		v.Set(a.NewCopyBytes((*tx.To).Bytes()))
	}

	v.Set(a.NewCopyBytes(tx.Input))

	// EIP155
	if chainID != 0 {
		v.Set(a.NewUint(chainID))
		v.Set(a.NewUint(0))
		v.Set(a.NewUint(0))
	}

	return v
}

func marshalRLPForDigest(a *fastrlp.Arena, tx *types.Transaction, chainID uint64) *fastrlp.Value {
	switch tp := tx.Payload.(type) {
	case *types.LegacyTransaction:
		return marshalLegacyTxRLPForDigest(a, tp, chainID)
	case *types.StateTransaction:
		return marshalStateTxRLPForDigest(a, tp, chainID)
	}

	return nil
}

// calcTxHash calculates the transaction hash (keccak256 hash of the RLP value)
func calcTxHash(tx *types.Transaction, chainID uint64) types.Hash {
	a := signerPool.Get()

	v := marshalRLPForDigest(a, tx, chainID)

	hash := keccak.Keccak256Rlp(nil, v)

	signerPool.Put(a)

	return types.BytesToHash(hash)
}

func wrapTxSignatures(tx *types.Transaction, v, r, s *big.Int) error {
	settableTxPayload, ok := tx.Payload.(types.SignatureSetterGetter)
	if !ok {
		return ErrUnsupportedTxSignatures
	}

	settableTxPayload.SetSignatureValues(v, r, s)

	return nil
}

func unwrapTxSignatures(tx *types.Transaction) (v, r, s *big.Int, err error) {
	settableTxPayload, ok := tx.Payload.(types.SignatureSetterGetter)
	if !ok {
		return nil, nil, nil, ErrUnsupportedTxSignatures
	}

	v, r, s = settableTxPayload.GetSignatureValues()

	return
}

// Hash is a wrapper function for the calcTxHash, with chainID 0
func (f *FrontierSigner) Hash(tx *types.Transaction) types.Hash {
	return calcTxHash(tx, 0)
}

// Magic numbers from Ethereum, used in v calculation
var (
	big27 = big.NewInt(27)
	big35 = big.NewInt(35)
)

// Sender decodes the signature and returns the sender of the transaction
func (f *FrontierSigner) Sender(tx *types.Transaction) (types.Address, error) {
	v, r, s, err := unwrapTxSignatures(tx)
	if err != nil {
		return types.Address{}, err
	}

	refV := big.NewInt(0)
	if v != nil {
		refV.SetBytes(v.Bytes())
	}

	refV.Sub(refV, big27)

	sig, err := encodeSignature(r, s, byte(refV.Int64()))
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

	err = wrapTxSignatures(
		tx,
		new(big.Int).SetBytes(f.CalculateV(sig[64])), // V
		new(big.Int).SetBytes(sig[:32]),              // R
		new(big.Int).SetBytes(sig[32:64]),            // S
	)

	if err != nil {
		return nil, err
	}

	return tx, nil
}

// calculateV returns the V value for transactions pre EIP155
func (f *FrontierSigner) CalculateV(parity byte) []byte {
	reference := big.NewInt(int64(parity))
	reference.Add(reference, big27)

	return reference.Bytes()
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

	v, r, s, err := unwrapTxSignatures(tx)
	if err != nil {
		return types.Address{}, err
	}

	// Check if v value conforms to an earlier standard (before EIP155)
	bigV := big.NewInt(0)
	if v != nil {
		bigV.SetBytes(v.Bytes())
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

	sig, err := encodeSignature(r, s, byte(bigV.Int64()))
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

	err = wrapTxSignatures(
		tx,
		new(big.Int).SetBytes(e.CalculateV(sig[64])), // V
		new(big.Int).SetBytes(sig[:32]),              // R
		new(big.Int).SetBytes(sig[32:64]),            // S
	)

	if err != nil {
		return nil, err
	}

	return tx, nil
}

// calculateV returns the V value for transaction signatures. Based on EIP155
func (e *EIP155Signer) CalculateV(parity byte) []byte {
	reference := big.NewInt(int64(parity))
	reference.Add(reference, big35)

	mulOperand := big.NewInt(0).Mul(big.NewInt(int64(e.chainID)), big.NewInt(2))

	reference.Add(reference, mulOperand)

	return reference.Bytes()
}

// encodeSignature generates a signature value based on the R, S and V value
func encodeSignature(R, S *big.Int, V byte) ([]byte, error) {
	if !ValidateSignatureValues(V, R, S) {
		return nil, fmt.Errorf("invalid txn signature")
	}

	sig := make([]byte, 65)
	copy(sig[32-len(R.Bytes()):32], R.Bytes())
	copy(sig[64-len(S.Bytes()):64], S.Bytes())
	sig[64] = V

	return sig, nil
}
