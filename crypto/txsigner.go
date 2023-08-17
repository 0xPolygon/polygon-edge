package crypto

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
)

// Magic numbers from Ethereum, used in v calculation
var (
	big27 = big.NewInt(27)
	big35 = big.NewInt(35)
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
		signer = NewEIP155Signer(chainID, forks.Homestead)
	} else {
		signer = NewFrontierSigner(forks.Homestead)
	}

	// London signer requires a fallback signer that is defined above.
	// This is the reason why the london signer check is separated.
	if forks.London {
		return NewLondonSigner(chainID, forks.Homestead, signer)
	}

	return signer
}

// encodeSignature generates a signature value based on the R, S and V value
func encodeSignature(R, S, V *big.Int, isHomestead bool) ([]byte, error) {
	if !ValidateSignatureValues(V, R, S, isHomestead) {
		return nil, fmt.Errorf("invalid txn signature")
	}

	sig := make([]byte, 65)
	copy(sig[32-len(R.Bytes()):32], R.Bytes())
	copy(sig[64-len(S.Bytes()):64], S.Bytes())
	sig[64] = byte(V.Int64()) // here is safe to convert it since ValidateSignatureValues will validate the v value

	return sig, nil
}

// calcTxHash calculates the transaction hash (keccak256 hash of the RLP value)
// LegacyTx:
// keccak256(RLP(nonce, gasPrice, gas, to, value, input, chainId, 0, 0))
// AccessListsTx:
// keccak256(RLP(type, chainId, nonce, gasPrice, gas, to, value, input, accessList))
// DynamicFeeTx:
// keccak256(RLP(type, chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, input, accessList))
func calcTxHash(tx *types.Transaction, chainID uint64) types.Hash {
	a := signerPool.Get()
	defer signerPool.Put(a)

	v := a.NewArray()

	if tx.Type != types.LegacyTx {
		v.Set(a.NewUint(chainID))
	}

	v.Set(a.NewUint(tx.Nonce))

	if tx.Type == types.DynamicFeeTx {
		v.Set(a.NewBigInt(tx.GasTipCap))
		v.Set(a.NewBigInt(tx.GasFeeCap))
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

	if tx.Type == types.LegacyTx {
		// EIP155
		if chainID != 0 {
			v.Set(a.NewUint(chainID))
			v.Set(a.NewUint(0))
			v.Set(a.NewUint(0))
		}
	} else {
		//nolint:godox
		// TODO: Introduce AccessList
		v.Set(a.NewArray())
	}

	var hash []byte
	if tx.Type == types.LegacyTx {
		hash = keccak.Keccak256Rlp(nil, v)
	} else {
		hash = keccak.PrefixedKeccak256Rlp([]byte{byte(tx.Type)}, nil, v)
	}

	return types.BytesToHash(hash)
}
