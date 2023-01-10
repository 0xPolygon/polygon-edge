package signer

import (
	"fmt"
	"math/big"

	bnsnark1 "github.com/0xPolygon/bnsnark1/core"
)

func bytesToBigInt2(bytes []byte) ([2]*big.Int, error) {
	if len(bytes) != 64 {
		return [2]*big.Int{}, fmt.Errorf("expect length 64 but got %d", len(bytes))
	}

	return [2]*big.Int{
		new(big.Int).SetBytes(reverse(bytes[:32])),
		new(big.Int).SetBytes(reverse(bytes[32:])),
	}, nil
}

func bytesToBigInt4(bytes []byte) ([4]*big.Int, error) {
	if len(bytes) != 128 {
		return [4]*big.Int{}, fmt.Errorf("expect length 128 but got %d", len(bytes))
	}

	return [4]*big.Int{
		new(big.Int).SetBytes(reverse(bytes[:32])),
		new(big.Int).SetBytes(reverse(bytes[32:64])),
		new(big.Int).SetBytes(reverse(bytes[64:96])),
		new(big.Int).SetBytes(reverse(bytes[96:])),
	}, nil
}

func bytesFromBigInt2(p [2]*big.Int) []byte {
	// zero or infinity
	if p[0].BitLen() == 0 && p[1].BitLen() == 0 {
		return bnsnark1.G1ToBytes(bnsnark1.G1Zero(new(bnsnark1.G1)))
	}

	bytes := make([]byte, 64)

	for i, v := range p {
		offset := i * 32

		reverse(v.FillBytes(bytes[offset : offset+32]))
	}

	return bytes
}

func bytesFromBigInt4(p [4]*big.Int) []byte {
	// zero or infinity
	if p[0].BitLen() == 0 && p[1].BitLen() == 0 && p[2].BitLen() == 0 && p[3].BitLen() == 0 {
		return bnsnark1.G2ToBytes(bnsnark1.G2Zero(new(bnsnark1.G2)))
	}

	bytes := make([]byte, 128)

	for i, v := range p {
		offset := i * 32

		reverse(v.FillBytes(bytes[offset : offset+32]))
	}

	return bytes
}

func reverse(s []byte) []byte {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}

	return s
}
