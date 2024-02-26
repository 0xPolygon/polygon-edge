package crypto

import (
	"crypto/ecdsa"
	"errors"
	"math/big"
	"math/bits"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
)

// EIP155Signer may be used for signing legacy (pre-EIP-155 and EIP-155) transactions
type EIP155Signer struct {
	*HomesteadSigner
	chainID uint64
}

// NewEIP155Signer returns new EIP155Signer object (constructor)
//
// EIP155Signer accepts the following types of transactions:
//   - EIP-155 replay protected transactions, and
//   - pre-EIP-155 legacy transactions
func NewEIP155Signer(chainID uint64) *EIP155Signer {
	return &EIP155Signer{
		chainID:         chainID,
		HomesteadSigner: NewHomesteadSigner(),
	}
}

// Hash returns the keccak256 hash of the transaction
//
// The EIP-155 transaction hash preimage is as follows:
// RLP(nonce, gasPrice, gas, to, value, input, chainId, 0, 0)
//
// Specification: https://eips.ethereum.org/EIPS/eip-155#specification
func (signer *EIP155Signer) Hash(tx *types.Transaction) types.Hash {
	RLP := arenaPool.Get()
	defer arenaPool.Put(RLP)

	// RLP(-, -, -, -, -, -, -, -, -)
	hashPreimage := RLP.NewArray()

	// RLP(nonce, -, -, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Nonce()))

	// RLP(nonce, gasPrice, -, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.GasPrice()))

	// RLP(nonce, gasPrice, gas, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Gas()))

	// Checking whether the transaction is a smart contract deployment
	if tx.To() == nil {
		// RLP(nonce, gasPrice, gas, to, -, -, -, -, -)
		hashPreimage.Set(RLP.NewNull())
	} else {
		// RLP(nonce, gasPrice, gas, to, -, -, -, -, -)
		hashPreimage.Set(RLP.NewCopyBytes((*(tx.To())).Bytes()))
	}

	// RLP(nonce, gasPrice, gas, to, value, -, -, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.Value()))

	// RLP(nonce, gasPrice, gas, to, value, input, -, -, -)
	hashPreimage.Set(RLP.NewCopyBytes(tx.Input()))

	// RLP(nonce, gasPrice, gas, to, value, input, chainId, -, -)
	hashPreimage.Set(RLP.NewUint(signer.chainID))

	// RLP(nonce, gasPrice, gas, to, value, input, chainId, 0, -)
	hashPreimage.Set(RLP.NewUint(0))

	// RLP(nonce, gasPrice, gas, to, value, input, chainId, 0, 0)
	hashPreimage.Set(RLP.NewUint(0))

	// keccak256(RLP(nonce, gasPrice, gas, to, value, input))
	hash := keccak.Keccak256Rlp(nil, hashPreimage)

	return types.BytesToHash(hash)
}

// Sender returns the sender of the transaction
func (signer *EIP155Signer) Sender(tx *types.Transaction) (types.Address, error) {
	if tx.Type() != types.LegacyTxType && tx.Type() != types.StateTxType {
		return types.ZeroAddress, types.ErrTxTypeNotSupported
	}

	protected := true

	v, r, s := tx.RawSignatureValues()

	// Checking one of the values is enought since they are inseparable
	if v == nil {
		return types.Address{}, errors.New("failed to recover sender, because signature is unknown")
	}

	bigV := big.NewInt(0).SetBytes(v.Bytes())

	if vv := v.Uint64(); bits.Len(uint(vv)) <= 8 {
		protected = vv != 27 && vv != 28
	}

	if !protected {
		return signer.HomesteadSigner.Sender(tx)
	}

	if err := validateTxChainID(tx, signer.chainID); err != nil {
		return types.ZeroAddress, err
	}

	// Reverse the V calculation to find the parity of the Y coordinate
	// v = CHAIN_ID * 2 + 35 + {0, 1} -> {0, 1} = v - 35 - CHAIN_ID * 2
	mulOperand := big.NewInt(0).Mul(big.NewInt(int64(signer.chainID)), big.NewInt(2))
	bigV.Sub(bigV, mulOperand)
	bigV.Sub(bigV, big35)

	return recoverAddress(signer.Hash(tx), r, s, bigV, true)
}

// SingTx takes the original transaction as input and returns its signed version
func (signer *EIP155Signer) SignTx(tx *types.Transaction, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	if tx.Type() != types.LegacyTxType && tx.Type() != types.StateTxType {
		return nil, types.ErrTxTypeNotSupported
	}

	tx = tx.Copy()

	hash := signer.Hash(tx)

	signature, err := Sign(privateKey, hash[:])
	if err != nil {
		return nil, err
	}

	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])

	if s.Cmp(secp256k1NHalf) > 0 {
		return nil, errors.New("SignTx method: S must be inclusively lower than secp256k1n/2")
	}

	v := new(big.Int).SetBytes(signer.calculateV(signature[64]))

	tx.SetSignatureValues(v, r, s)

	return tx, nil
}

// Private method calculateV returns the V value for the EIP-155 transactions
//
// V is calculated by the formula: {0, 1} + CHAIN_ID * 2 + 35 where {0, 1} denotes the parity of the Y coordinate
func (signer *EIP155Signer) calculateV(parity byte) []byte {
	// a = {0, 1} + 35
	a := big.NewInt(int64(parity))
	a.Add(a, big35)

	// b = CHAIN_ID * 2
	b := big.NewInt(0).Mul(big.NewInt(int64(signer.chainID)), big.NewInt(2))

	a.Add(a, b)

	return a.Bytes()
}
