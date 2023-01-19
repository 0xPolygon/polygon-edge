package crypto

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/keccak"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
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
		signer = NewEIP155Signer(chainID)
	} else if forks.London {
		signer = NewLondonSigner(chainID)
	} else {
		signer = &FrontierSigner{}
	}

	return signer
}

// calcTxHash calculates the transaction hash (keccak256 hash of the RLP value)
func calcTxHash(tx *types.Transaction, chainID uint64) types.Hash {
	a := signerPool.Get()

	v := a.NewArray()
	v.Set(a.NewUint(tx.Nonce))

	if tx.Type == types.DynamicFeeTx {
		v.Set(a.NewBigInt(tx.GasFeeCap))
		v.Set(a.NewBigInt(tx.GasTipCap))
	} else {
		v.Set(a.NewBigInt(tx.GasPrice))
	}

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
