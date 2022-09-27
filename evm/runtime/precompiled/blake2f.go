package precompiled

import (
	"encoding/binary"
	"fmt"
	"math/bits"

	"github.com/0xPolygon/polygon-edge/chain"
)

type blake2f struct {
	p *Precompiled
}

func (e *blake2f) gas(input []byte, config *chain.ForksInTime) uint64 {
	if len(input) != 213 {
		return 0
	}

	return uint64(binary.BigEndian.Uint32(input[0:4]))
}

func (e *blake2f) run(input []byte) ([]byte, error) {
	// validate input
	if len(input) != 213 {
		return nil, fmt.Errorf("bad length")
	}

	if lastByte := input[212]; lastByte != 0 && lastByte != 1 {
		return nil, fmt.Errorf("bad flag")
	}

	// rounds (first 4 bytes)
	rounds := binary.BigEndian.Uint32(input[:4])
	input = input[4:]

	// h. Next 64 bytes in groups of uint64 (8)
	h := [8]uint64{}
	for i := 0; i < 8; i++ {
		h[i] = binary.LittleEndian.Uint64(input[:8])
		input = input[8:]
	}

	// m. Next 128 bytes in group of uint64 (16)
	m := [16]uint64{}
	for i := 0; i < 16; i++ {
		m[i] = binary.LittleEndian.Uint64(input[:8])
		input = input[8:]
	}

	// c. Two 8 bytes
	c := [2]uint64{}
	c[0] = binary.LittleEndian.Uint64(input[:8])
	c[1] = binary.LittleEndian.Uint64(input[8:16])

	// flag. Last byte
	flag := input[16]

	F(&h, m, c, flag == 1, rounds)

	res := make([]byte, 64)

	for i := 0; i < 8; i++ {
		o := i * 8
		binary.LittleEndian.PutUint64(res[o:o+8], h[i])
	}

	return res, nil
}

// TODO: Move this to own repo and include the assembly code from the Go repo
// Copied from keep-network/blake2f

// IV is an initialization vector for BLAKE2b
var IV = [8]uint64{
	0x6a09e667f3bcc908, 0xbb67ae8584caa73b, 0x3c6ef372fe94f82b, 0xa54ff53a5f1d36f1,
	0x510e527fade682d1, 0x9b05688c2b3e6c1f, 0x1f83d9abfb41bd6b, 0x5be0cd19137e2179,
}

// the precomputed values for BLAKE2b
// there are 10 16-byte arrays - one for each round
// the entries are calculated from the sigma constants.
var precomputed = [10][16]byte{
	{0, 2, 4, 6, 1, 3, 5, 7, 8, 10, 12, 14, 9, 11, 13, 15},
	{14, 4, 9, 13, 10, 8, 15, 6, 1, 0, 11, 5, 12, 2, 7, 3},
	{11, 12, 5, 15, 8, 0, 2, 13, 10, 3, 7, 9, 14, 6, 1, 4},
	{7, 3, 13, 11, 9, 1, 12, 14, 2, 5, 4, 15, 6, 10, 0, 8},
	{9, 5, 2, 10, 0, 7, 4, 15, 14, 11, 6, 3, 1, 12, 8, 13},
	{2, 6, 0, 8, 12, 10, 11, 3, 4, 7, 15, 1, 13, 5, 14, 9},
	{12, 1, 14, 4, 5, 15, 13, 10, 0, 6, 9, 8, 7, 3, 2, 11},
	{13, 7, 12, 3, 11, 14, 1, 9, 5, 15, 8, 2, 0, 4, 6, 10},
	{6, 14, 11, 0, 15, 9, 3, 8, 12, 13, 1, 10, 2, 7, 4, 5},
	{10, 8, 7, 1, 2, 4, 6, 5, 15, 9, 3, 13, 11, 14, 12, 0},
}

// F is a compression function for BLAKE2b. It takes as an argument the state
// vector `h`, message block vector `m`, offset counter `t`, final
// block indicator flag `f`, and number of rounds `rounds`. The state vector
// provided as the first parameter is modified by the function.
func F(h *[8]uint64, m [16]uint64, c [2]uint64, f bool, rounds uint32) {
	c0, c1 := c[0], c[1]

	v0, v1, v2, v3, v4, v5, v6, v7 := h[0], h[1], h[2], h[3], h[4], h[5], h[6], h[7]
	v8, v9, v10, v11, v12, v13, v14, v15 := IV[0], IV[1], IV[2], IV[3], IV[4], IV[5], IV[6], IV[7]
	v12 ^= c0
	v13 ^= c1

	if f {
		v14 ^= 0xffffffffffffffff
	}

	for j := uint32(0); j < rounds; j++ {
		s := &(precomputed[j%10])

		v0 += m[s[0]]
		v0 += v4
		v12 ^= v0
		v12 = bits.RotateLeft64(v12, -32)
		v8 += v12
		v4 ^= v8
		v4 = bits.RotateLeft64(v4, -24)
		v1 += m[s[1]]
		v1 += v5
		v13 ^= v1
		v13 = bits.RotateLeft64(v13, -32)
		v9 += v13
		v5 ^= v9
		v5 = bits.RotateLeft64(v5, -24)
		v2 += m[s[2]]
		v2 += v6
		v14 ^= v2
		v14 = bits.RotateLeft64(v14, -32)
		v10 += v14
		v6 ^= v10
		v6 = bits.RotateLeft64(v6, -24)
		v3 += m[s[3]]
		v3 += v7
		v15 ^= v3
		v15 = bits.RotateLeft64(v15, -32)
		v11 += v15
		v7 ^= v11
		v7 = bits.RotateLeft64(v7, -24)

		v0 += m[s[4]]
		v0 += v4
		v12 ^= v0
		v12 = bits.RotateLeft64(v12, -16)
		v8 += v12
		v4 ^= v8
		v4 = bits.RotateLeft64(v4, -63)
		v1 += m[s[5]]
		v1 += v5
		v13 ^= v1
		v13 = bits.RotateLeft64(v13, -16)
		v9 += v13
		v5 ^= v9
		v5 = bits.RotateLeft64(v5, -63)
		v2 += m[s[6]]
		v2 += v6
		v14 ^= v2
		v14 = bits.RotateLeft64(v14, -16)
		v10 += v14
		v6 ^= v10
		v6 = bits.RotateLeft64(v6, -63)
		v3 += m[s[7]]
		v3 += v7
		v15 ^= v3
		v15 = bits.RotateLeft64(v15, -16)
		v11 += v15
		v7 ^= v11
		v7 = bits.RotateLeft64(v7, -63)

		v0 += m[s[8]]
		v0 += v5
		v15 ^= v0
		v15 = bits.RotateLeft64(v15, -32)
		v10 += v15
		v5 ^= v10
		v5 = bits.RotateLeft64(v5, -24)
		v1 += m[s[9]]
		v1 += v6
		v12 ^= v1
		v12 = bits.RotateLeft64(v12, -32)
		v11 += v12
		v6 ^= v11
		v6 = bits.RotateLeft64(v6, -24)
		v2 += m[s[10]]
		v2 += v7
		v13 ^= v2
		v13 = bits.RotateLeft64(v13, -32)
		v8 += v13
		v7 ^= v8
		v7 = bits.RotateLeft64(v7, -24)
		v3 += m[s[11]]
		v3 += v4
		v14 ^= v3
		v14 = bits.RotateLeft64(v14, -32)
		v9 += v14
		v4 ^= v9
		v4 = bits.RotateLeft64(v4, -24)

		v0 += m[s[12]]
		v0 += v5
		v15 ^= v0
		v15 = bits.RotateLeft64(v15, -16)
		v10 += v15
		v5 ^= v10
		v5 = bits.RotateLeft64(v5, -63)
		v1 += m[s[13]]
		v1 += v6
		v12 ^= v1
		v12 = bits.RotateLeft64(v12, -16)
		v11 += v12
		v6 ^= v11
		v6 = bits.RotateLeft64(v6, -63)
		v2 += m[s[14]]
		v2 += v7
		v13 ^= v2
		v13 = bits.RotateLeft64(v13, -16)
		v8 += v13
		v7 ^= v8
		v7 = bits.RotateLeft64(v7, -63)
		v3 += m[s[15]]
		v3 += v4
		v14 ^= v3
		v14 = bits.RotateLeft64(v14, -16)
		v9 += v14
		v4 ^= v9
		v4 = bits.RotateLeft64(v4, -63)
	}

	h[0] ^= v0 ^ v8
	h[1] ^= v1 ^ v9
	h[2] ^= v2 ^ v10
	h[3] ^= v3 ^ v11
	h[4] ^= v4 ^ v12
	h[5] ^= v5 ^ v13
	h[6] ^= v6 ^ v14
	h[7] ^= v7 ^ v15
}
