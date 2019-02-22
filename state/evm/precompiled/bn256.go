package precompiled

import (
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/crypto/bn256"
	"github.com/umbracle/minimal/helper"
)

// newCurvePoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newCurvePoint(blob []byte) (*bn256.G1, error) {
	p := new(bn256.G1)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// newTwistPoint unmarshals a binary blob into a bn256 elliptic curve point,
// returning it, or an error if the point is invalid.
func newTwistPoint(blob []byte) (*bn256.G2, error) {
	p := new(bn256.G2)
	if _, err := p.Unmarshal(blob); err != nil {
		return nil, err
	}
	return p, nil
}

// bn256Add implements a native elliptic curve point addition.
type bn256Add struct {
	Base uint64
}

func (c *bn256Add) Gas(input []byte) uint64 {
	return c.Base
}

func (c *bn256Add) Call(input []byte) ([]byte, error) {
	x, err := newCurvePoint(helper.GetData(input, 0, 64))
	if err != nil {
		return nil, err
	}
	y, err := newCurvePoint(helper.GetData(input, 64, 64))
	if err != nil {
		return nil, err
	}
	res := new(bn256.G1)
	res.Add(x, y)
	return res.Marshal(), nil
}

// bn256ScalarMul implements a native elliptic curve scalar multiplication.
type bn256ScalarMul struct {
	Base uint64
}

func (c *bn256ScalarMul) Gas(input []byte) uint64 {
	return c.Base
}

func (c *bn256ScalarMul) Call(input []byte) ([]byte, error) {
	p, err := newCurvePoint(helper.GetData(input, 0, 64))
	if err != nil {
		return nil, err
	}
	res := new(bn256.G1)
	res.ScalarMult(p, new(big.Int).SetBytes(helper.GetData(input, 64, 32)))
	return res.Marshal(), nil
}

// true32Byte is returned if the bn256 pairing check succeeds.
var true32Byte = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

// false32Byte is returned if the bn256 pairing check fails.
var false32Byte = make([]byte, 32)

// errBadPairingInput is returned if the bn256 pairing input is invalid.
var errBadPairingInput = errors.New("bad elliptic curve pairing size")

// bn256Pairing implements a pairing pre-compile for the bn256 curve
type bn256Pairing struct {
	Base uint64
	Pair uint64
}

// RequiredGas returns the gas required to execute the pre-compiled contract.
func (c *bn256Pairing) Gas(input []byte) uint64 {
	return c.Base + uint64(len(input)/192)*c.Pair
}

func (c *bn256Pairing) Call(input []byte) ([]byte, error) {
	// Handle some corner cases cheaply
	if len(input)%192 > 0 {
		return nil, errBadPairingInput
	}
	// Convert the input into a set of coordinates
	var (
		cs []*bn256.G1
		ts []*bn256.G2
	)
	for i := 0; i < len(input); i += 192 {
		c, err := newCurvePoint(input[i : i+64])
		if err != nil {
			return nil, err
		}
		t, err := newTwistPoint(input[i+64 : i+192])
		if err != nil {
			return nil, err
		}
		cs = append(cs, c)
		ts = append(ts, t)
	}
	// Execute the pairing checks and return the results
	if bn256.PairingCheck(cs, ts) {
		return true32Byte, nil
	}
	return false32Byte, nil
}
