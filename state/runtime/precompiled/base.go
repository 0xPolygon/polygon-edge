package precompiled

import (
	"crypto/sha256"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck
	"math/big"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/keccak"
)

type ecrecover struct {
	p *Precompiled
}

func (e *ecrecover) gas(input []byte, config *chain.ForksInTime) uint64 {
	return 3000
}

func (e *ecrecover) run(input []byte) ([]byte, error) {
	input, _ = e.p.get(input, 128)

	// recover the value v. Expect all zeros except the last byte
	for i := 32; i < 63; i++ {
		if input[i] != 0 {
			return nil, nil
		}
	}

	v := input[63] - 27
	r := big.NewInt(0).SetBytes(input[64:96])
	s := big.NewInt(0).SetBytes(input[96:128])

	if !crypto.ValidateSignatureValues(v, r, s) {
		return nil, nil
	}

	pubKey, err := crypto.Ecrecover(input[:32], append(input[64:128], v))
	if err != nil {
		return nil, nil
	}

	dst := keccak.Keccak256(nil, pubKey[1:])
	dst = e.p.leftPad(dst[12:], 32)

	return dst, nil
}

type identity struct {
}

func (i *identity) gas(input []byte, config *chain.ForksInTime) uint64 {
	return baseGasCalc(input, 15, 3)
}

func (i *identity) run(in []byte) ([]byte, error) {
	return in, nil
}

type sha256h struct {
}

func (s *sha256h) gas(input []byte, config *chain.ForksInTime) uint64 {
	return baseGasCalc(input, 60, 12)
}

func (s *sha256h) run(input []byte) ([]byte, error) {
	h := sha256.Sum256(input)

	return h[:], nil
}

type ripemd160h struct {
	p *Precompiled
}

func (r *ripemd160h) gas(input []byte, config *chain.ForksInTime) uint64 {
	return baseGasCalc(input, 600, 120)
}

func (r *ripemd160h) run(input []byte) ([]byte, error) {
	ripemd := ripemd160.New()
	ripemd.Write(input)
	res := ripemd.Sum(nil)

	return r.p.leftPad(res, 32), nil
}

func baseGasCalc(input []byte, base, word uint64) uint64 {
	return base + uint64(len(input)+31)/32*word
}
