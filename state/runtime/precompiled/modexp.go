package precompiled

import (
	"math/big"

	"math"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

type modExp struct {
	p *Precompiled
}

var (
	big1      = big.NewInt(1)
	big4      = big.NewInt(4)
	big8      = big.NewInt(8)
	big16     = big.NewInt(16)
	big32     = big.NewInt(32)
	big64     = big.NewInt(64)
	big96     = big.NewInt(96)
	big480    = big.NewInt(480)
	big1024   = big.NewInt(1024)
	big3072   = big.NewInt(3072)
	big199680 = big.NewInt(199680)
)

var (
	divisor = big.NewInt(20)
)

func adjustedExponentLength(expLen, head *big.Int) *big.Int {
	bitlength := uint64(0)
	if head.Sign() != 0 {
		bitlength = uint64(head.BitLen() - 1)
	}

	if expLen.Cmp(big32) <= 0 {
		// return the index of the highest bit
		return new(big.Int).SetUint64(bitlength)
	}

	head.Sub(expLen, big32)
	head.Mul(head, big8)
	head.Add(head, new(big.Int).SetUint64(bitlength))

	return head
}

func subMul(x, a, b, c *big.Int) *big.Int {
	// x ** 2 // a + b * x - c
	tmp := new(big.Int)

	// x ** 2 / a
	tmp.Mul(x, x)
	tmp.Div(tmp, a)

	// b * x - c
	x.Mul(x, b)
	x.Sub(x, c)

	return x.Add(x, tmp)
}

func multComplexity(x *big.Int) *big.Int {
	if x.Cmp(big64) <= 0 {
		// x ** x
		x.Mul(x, x)
	} else if x.Cmp(big1024) <= 0 {
		// x ** 2 // 4 + 96 * x - 3072
		x = subMul(x, big4, big96, big3072)
	} else {
		// x ** 2 // 16 + 480 * x - 199680
		x = subMul(x, big16, big480, big199680)
	}

	return x
}

func (m *modExp) gas(input []byte, config *chain.ForksInTime) uint64 {
	var val, tail []byte

	val, tail = m.p.get(input, 32)
	baseLen := new(big.Int).SetBytes(val)

	val, tail = m.p.get(tail, 32)
	expLen := new(big.Int).SetBytes(val)

	val, _ = m.p.get(tail, 32)
	modLen := new(big.Int).SetBytes(val)

	if len(input) > 96 {
		input = input[96:]
	} else {
		input = input[:0]
	}

	expHeadLen := uint64(32)
	if expLen.Cmp(big32) < 0 {
		expHeadLen = expLen.Uint64()
	}

	expHead := new(big.Int)

	if bLen := baseLen.Uint64(); bLen < uint64(len(input)) {
		val, _ = m.p.get(input[bLen:], int(expHeadLen))
		expHead.SetBytes(val)
	}

	// a := mult_complexity(max(length_of_MODULUS, length_of_BASE)
	gasCost := new(big.Int)
	if modLen.Cmp(baseLen) >= 0 {
		gasCost.Set(modLen)
	} else {
		gasCost.Set(baseLen)
	}

	gasCost = multComplexity(gasCost)

	// a = a * max(ADJUSTED_EXPONENT_LENGTH, 1)
	adjExpLen := adjustedExponentLength(expLen, expHead)
	if adjExpLen.Cmp(big1) >= 0 {
		gasCost.Mul(gasCost, adjExpLen)
	} else {
		gasCost.Mul(gasCost, big1)
	}

	// a = a / div
	gasCost.Div(gasCost, divisor)

	// cap to the max uint64
	if !gasCost.IsUint64() {
		return math.MaxUint64
	}

	return gasCost.Uint64()
}

func (m *modExp) run(input []byte, _ types.Address, _ runtime.Host) ([]byte, error) {
	// get the lengths
	var baseLen, exponentLen, modulusLen uint64

	baseLen, input = m.p.getUint64(input)
	exponentLen, input = m.p.getUint64(input)
	modulusLen, input = m.p.getUint64(input)

	if baseLen == 0 && modulusLen == 0 {
		return nil, nil
	}

	// get the values
	var val []byte

	val, input = m.p.get(input, int(baseLen))
	base := new(big.Int).SetBytes(val)

	val, input = m.p.get(input, int(exponentLen))
	exponent := new(big.Int).SetBytes(val)

	val, _ = m.p.get(input, int(modulusLen))
	modulus := new(big.Int).SetBytes(val)

	var res []byte
	if modulus.Sign() != 0 {
		res = base.Exp(base, exponent, modulus).Bytes()
	}

	return m.p.leftPad(res, int(modulusLen)), nil
}
