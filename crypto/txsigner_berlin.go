package crypto

import (
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/keccak"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// BerlinSigner may be used for signing legacy (pre-EIP-155 and EIP-155) and EIP-2930 transactions
type BerlinSigner struct {
	*EIP155Signer
}

// NewBerlinSigner returns new BerlinSinger object (constructor)
//
// BerlinSigner accepts the following types of transactions:
//   - EIP-2930 access list transactions,
//   - EIP-155 replay protected transactions, and
//   - pre-EIP-155 legacy transactions
func NewBerlinSigner(chainID uint64) *BerlinSigner {
	return &BerlinSigner{EIP155Signer: NewEIP155Signer(chainID)}
}

// Hash returns the keccak256 hash of the transaction
//
// The EIP-2930 transaction hash preimage is as follows:
// (0x01 || RLP(chainId, nonce, gasPrice, gas, to, value, input, accessList)
//
// Specification: https://eips.ethereum.org/EIPS/eip-2930#specification
func (signer *BerlinSigner) Hash(tx *types.Transaction) types.Hash {
	if tx.Type() != types.AccessListTxType {
		return signer.EIP155Signer.Hash(tx)
	}

	RLP := arenaPool.Get()
	defer arenaPool.Put(RLP)

	// RLP(-, -, -, -, -, -, -, -)
	hashPreimage := RLP.NewArray()

	// RLP(chainId, -, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(signer.chainID))

	// RLP(chainId, nonce, -, -, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Nonce()))

	// RLP(chainId, nonce, gasPrice, -, -, -, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.GasPrice()))

	// RLP(chainId, nonce, gasPrice, gas, -, -, -, -)
	hashPreimage.Set(RLP.NewUint(tx.Gas()))

	// Checking whether the transaction is a smart contract deployment
	if tx.To() == nil {
		// RLP(chainId, nonce, gasPrice, gas, to, -, -, -)
		hashPreimage.Set(RLP.NewNull())
	} else {
		// RLP(chainId, nonce, gasPrice, gas, to, -, -, -)
		hashPreimage.Set(RLP.NewCopyBytes((*(tx.To())).Bytes()))
	}

	// RLP(chainId, nonce, gasPrice, gas, to, value, -, -)
	hashPreimage.Set(RLP.NewBigInt(tx.Value()))

	// RLP(chainId, nonce, gasPrice, gas, to, value, input, -)
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

	// RLP(chainId, nonce, gasPrice, gas, to, value, input, accessList)
	hashPreimage.Set(rlpAccessList)

	// keccak256(0x01 || RLP(chainId, nonce, gasPrice, gas, to, value, input, accessList)
	hash := keccak.PrefixedKeccak256Rlp([]byte{byte(tx.Type())}, nil, hashPreimage)

	return types.BytesToHash(hash)
}

// Sender returns the sender of the transaction
func (signer *BerlinSigner) Sender(tx *types.Transaction) (types.Address, error) {
	if tx.Type() == types.DynamicFeeTxType {
		return types.ZeroAddress, types.ErrTxTypeNotSupported
	}

	if tx.Type() != types.AccessListTxType {
		return signer.EIP155Signer.Sender(tx)
	}

	v, r, s := tx.RawSignatureValues()

	// Checking one of the values is enough since they are inseparable
	if v == nil {
		return types.Address{}, errors.New("failed to recover sender, because signature is unknown")
	}

	if err := validateTxChainID(tx, signer.chainID); err != nil {
		return types.ZeroAddress, err
	}

	return recoverAddress(signer.Hash(tx), r, s, v, true)
}

// SignTx takes the original transaction as input and returns its signed version
func (signer *BerlinSigner) SignTx(tx *types.Transaction, privateKey *ecdsa.PrivateKey) (*types.Transaction, error) {
	if tx.Type() == types.DynamicFeeTxType {
		return nil, types.ErrTxTypeNotSupported
	}

	if tx.Type() != types.AccessListTxType {
		return signer.EIP155Signer.SignTx(tx, privateKey)
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

// Private method calculateV returns the V value for the EIP-2930 transactions
//
// V represents the parity of the Y coordinate
func (signer *BerlinSigner) calculateV(parity byte) []byte {
	return big.NewInt(int64(parity)).Bytes()
}

func (signer *BerlinSigner) SignTxWithCallBack(tx *types.Transaction,
	signFn func(hash types.Hash) (sig []byte, err error)) (*types.Transaction, error) {
	if tx.Type() != types.AccessListTxType {
		return signer.EIP155Signer.SignTxWithCallback(tx, signFn)
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
