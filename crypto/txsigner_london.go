package crypto

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// LondonSigner may be used for signing legacy (pre-EIP-155 and EIP-155), EIP-2930 and EIP-1559 transactions
type LondonSigner struct {
	*BerlinSigner
}

// NewLondonSigner returns new LondonSinger object (constructor)
//
// LondonSigner accepts the following types of transactions:
//   - EIP-1559 dynamic fee transactions
//   - EIP-2930 access list transactions,
//   - EIP-155 replay protected transactions, and
//   - pre-EIP-155 legacy transactions
func NewLondonSigner(chainID uint64) *LondonSigner {
	return &LondonSigner{
		BerlinSigner: NewBerlinSigner(chainID),
	}
}

// Hash returns the keccak256 hash of the transaction
//
// The EIP-1559 transaction hash preimage is as follows:
// (0x02 || RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, input, accessList)
//
// Specification: https://eips.ethereum.org/EIPS/eip-1559#specification
func (signer *LondonSigner) Hash(tx *types.Transaction) types.Hash {
	if tx.Type() != types.DynamicFeeTxType {
		return signer.BerlinSigner.Hash(tx)
	}

	RLP := arenaPool.Get()
	defer arenaPool.Put(RLP)

	// RLP(-, -, -, -, -, -, -, -, -)
	hashPreimage := RLP.NewArray()

	// RLP(chainId, -, -, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(signer.chainID))

	// RLP(chainId, nonce, -, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Nonce()))

	// RLP(chainId, nonce, gasTipCap, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.GasTipCap()))

	// RLP(chainId, nonce, gasTipCap, gasFeeCap, -, -, -, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.GasFeeCap()))

	// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Gas()))

	// Checking whether the transaction is a smart contract deployment
	if tx.To() == nil {
		// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, -, -, -)
		hashPreimage.Set(RLP.NewNull())
	} else {
		// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, -, -, -)
		hashPreimage.Set(RLP.NewCopyBytes((*(tx.To())).Bytes()))
	}

	// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.Value()))

	// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, input, -)
	hashPreimage.Set(RLP.NewCopyBytes(tx.Input()))

	// Serialization format of the access list:
	// [[{20-bytes address}, [{32-bytes key}, ...]], ...] where `...` denotes zero or more items
	var rlpAccessList *fastrlp.Value

	accessList := tx.AccessList()
	if accessList != nil {
		rlpAccessList = accessList.MarshallRLPWith(RLP)
	} else {
		rlpAccessList = RLP.NewArray()
	}

	// RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, input,accessList)
	hashPreimage.Set(rlpAccessList)

	// keccak256(0x02 || RLP(chainId, nonce, gasTipCap, gasFeeCap, gas, to, value, input,accessList)
	hash := keccak.PrefixedKeccak256Rlp([]byte{byte(tx.Type())}, nil, hashPreimage)

	return types.BytesToHash(hash)
}

// Sender returns the sender of the transaction
func (signer *LondonSigner) Sender(tx *types.Transaction) (types.Address, error) {
	if tx.Type() != types.DynamicFeeTxType {
		return signer.BerlinSigner.Sender(tx)
	}

	v, r, s := tx.RawSignatureValues()

	// Checking one of the values is enought since they are inseparable
	if v == nil {
		return types.Address{}, errors.New("failed to recover sender, because signature is unknown")
	}

	if err := validateTxChainID(tx, signer.chainID); err != nil {
		return types.ZeroAddress, err
	}

	return recoverAddress(signer.Hash(tx), r, s, v, true)
}

// SignTx takes the original transaction as input and returns its signed version
func (signer *LondonSigner) SignTx(tx *types.Transaction, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	if tx.Type() != types.DynamicFeeTxType {
		return signer.BerlinSigner.SignTx(tx, privateKey)
	}

	tx = tx.Copy()
	h := signer.Hash(tx)

	sig, err := Sign(privateKey, h[:])
	if err != nil {
		return nil, err
	}

	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:64])

	if s.Cmp(secp256k1NHalf) > 0 {
		return nil, errors.New("SignTx method: S must be inclusively lower than secp256k1n/2")
	}

	v := new(big.Int).SetBytes(signer.calculateV(sig[64]))

	tx.SetSignatureValues(v, r, s)

	return tx, nil
}

func (signer *LondonSigner) SignTxWithCallback(tx *types.Transaction,
	signFn func(hash types.Hash) (sig []byte, err error)) (*types.Transaction, error) {
	if tx.Type() != types.DynamicFeeTxType {
		return signer.BerlinSigner.SignTxWithCallback(tx, signFn)
	}

	tx = tx.Copy()
	h := signer.Hash(tx)

	signature, err := signFn(h)
	if err != nil {
		return nil, err
	}

	tx.SplitToRawSignatureValues(signature, signer.calculateV(signature[64]))

	return tx, nil
}
