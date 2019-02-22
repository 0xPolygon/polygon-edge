package precompiled

import (
	"crypto/sha256"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/ripemd160"
)

type ecrecover struct {
	Base uint64
}

func (c *ecrecover) Gas(input []byte) uint64 {
	return c.Base
}

func (c *ecrecover) Call(input []byte) ([]byte, error) {
	const ecRecoverInputLength = 128

	input = common.RightPadBytes(input, ecRecoverInputLength)
	// "input" is (hash, v, r, s), each 32 bytes
	// but for ecrecover we want (r, s, v)

	r := new(big.Int).SetBytes(input[64:96])
	s := new(big.Int).SetBytes(input[96:128])
	v := input[63] - 27

	// tighter sig s values input homestead only apply to tx sigs
	if !allZero(input[32:63]) || !crypto.ValidateSignatureValues(v, r, s, false) {
		return nil, nil
	}
	// v needs to be at the end for libsecp256k1
	pubKey, err := crypto.Ecrecover(input[:32], append(input[64:128], v))
	// make sure the public key is a valid one
	if err != nil {
		return nil, nil
	}

	// the first byte of pubkey is bitcoin heritage
	return common.LeftPadBytes(crypto.Keccak256(pubKey[1:])[12:], 32), nil
}

func allZero(b []byte) bool {
	for _, byte := range b {
		if byte != 0 {
			return false
		}
	}
	return true
}

type dataCopy struct {
	Base uint64
	Word uint64
}

func (c *dataCopy) Gas(input []byte) uint64 {
	return uint64(len(input)+31)/32*c.Word + c.Base
}

func (c *dataCopy) Call(in []byte) ([]byte, error) {
	return in, nil
}

// SHA256 implemented as a native contract.
type sha256hash struct {
	Base uint64
	Word uint64
}

func (s *sha256hash) Gas(input []byte) uint64 {
	return uint64(len(input)+31)/32*s.Word + s.Base
}

func (c *sha256hash) Call(input []byte) ([]byte, error) {
	h := sha256.Sum256(input)
	return h[:], nil
}

// RIPEMD160 implemented as a native contract.
type ripemd160hash struct {
	Base uint64
	Word uint64
}

func (c *ripemd160hash) Gas(input []byte) uint64 {
	return uint64(len(input)+31)/32*c.Word + c.Base
}

func (c *ripemd160hash) Call(input []byte) ([]byte, error) {
	ripemd := ripemd160.New()
	ripemd.Write(input)
	return common.LeftPadBytes(ripemd.Sum(nil), 32), nil
}
