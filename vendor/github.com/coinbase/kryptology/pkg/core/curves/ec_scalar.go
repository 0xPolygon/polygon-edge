//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/sha512"
	"fmt"
	"io"
	"math/big"

	"filippo.io/edwards25519"
	"github.com/btcsuite/btcd/btcec"
	"github.com/bwesterb/go-ristretto"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core"
	"github.com/coinbase/kryptology/pkg/core/curves/native/bls12381"
)

type EcScalar interface {
	Add(x, y *big.Int) *big.Int
	Sub(x, y *big.Int) *big.Int
	Neg(x *big.Int) *big.Int
	Mul(x, y *big.Int) *big.Int
	Hash(input []byte) *big.Int
	Div(x, y *big.Int) *big.Int
	Random() (*big.Int, error)
	IsValid(x *big.Int) bool
	Bytes(x *big.Int) []byte // fixed-length byte array
}

type K256Scalar struct{}

// Static interface assertion
var _ EcScalar = (*K256Scalar)(nil)

// warning: the Euclidean alg which Mod uses is not constant-time.

func NewK256Scalar() *K256Scalar {
	return &K256Scalar{}
}

func (k K256Scalar) Add(x, y *big.Int) *big.Int {
	v := new(big.Int).Add(x, y)
	v.Mod(v, btcec.S256().N)
	return v
}

func (k K256Scalar) Sub(x, y *big.Int) *big.Int {
	v := new(big.Int).Sub(x, y)
	v.Mod(v, btcec.S256().N)
	return v
}

func (k K256Scalar) Neg(x *big.Int) *big.Int {
	v := new(big.Int).Sub(btcec.S256().N, x)
	v.Mod(v, btcec.S256().N)
	return v
}

func (k K256Scalar) Mul(x, y *big.Int) *big.Int {
	v := new(big.Int).Mul(x, y)
	v.Mod(v, btcec.S256().N)
	return v
}

func (k K256Scalar) Div(x, y *big.Int) *big.Int {
	t := new(big.Int).ModInverse(y, btcec.S256().N)
	return k.Mul(x, t)
}

func (k K256Scalar) Hash(input []byte) *big.Int {
	return new(ScalarK256).Hash(input).BigInt()
}

func (k K256Scalar) Random() (*big.Int, error) {
	b := make([]byte, 48)
	n, err := crand.Read(b)
	if err != nil {
		return nil, err
	}
	if n != 48 {
		return nil, fmt.Errorf("insufficient bytes read")
	}
	v := new(big.Int).SetBytes(b)
	v.Mod(v, btcec.S256().N)
	return v, nil
}

func (k K256Scalar) IsValid(x *big.Int) bool {
	return core.In(x, btcec.S256().N) == nil
}

func (k K256Scalar) Bytes(x *big.Int) []byte {
	bytes := make([]byte, 32)
	x.FillBytes(bytes) // big-endian; will left-pad.
	return bytes
}

type P256Scalar struct{}

// Static interface assertion
var _ EcScalar = (*P256Scalar)(nil)

func NewP256Scalar() *P256Scalar {
	return &P256Scalar{}
}

func (k P256Scalar) Add(x, y *big.Int) *big.Int {
	v := new(big.Int).Add(x, y)
	v.Mod(v, elliptic.P256().Params().N)
	return v
}

func (k P256Scalar) Sub(x, y *big.Int) *big.Int {
	v := new(big.Int).Sub(x, y)
	v.Mod(v, elliptic.P256().Params().N)
	return v
}

func (k P256Scalar) Neg(x *big.Int) *big.Int {
	v := new(big.Int).Sub(elliptic.P256().Params().N, x)
	v.Mod(v, elliptic.P256().Params().N)
	return v
}

func (k P256Scalar) Mul(x, y *big.Int) *big.Int {
	v := new(big.Int).Mul(x, y)
	v.Mod(v, elliptic.P256().Params().N)
	return v
}

func (k P256Scalar) Div(x, y *big.Int) *big.Int {
	t := new(big.Int).ModInverse(y, elliptic.P256().Params().N)
	return k.Mul(x, t)
}

func (k P256Scalar) Hash(input []byte) *big.Int {
	return new(ScalarP256).Hash(input).BigInt()
}

func (k P256Scalar) Random() (*big.Int, error) {
	b := make([]byte, 48)
	n, err := crand.Read(b)
	if err != nil {
		return nil, err
	}
	if n != 48 {
		return nil, fmt.Errorf("insufficient bytes read")
	}
	v := new(big.Int).SetBytes(b)
	v.Mod(v, elliptic.P256().Params().N)
	return v, nil
}

func (k P256Scalar) IsValid(x *big.Int) bool {
	return core.In(x, elliptic.P256().Params().N) == nil
}

func (k P256Scalar) Bytes(x *big.Int) []byte {
	bytes := make([]byte, 32)
	x.FillBytes(bytes) // big-endian; will left-pad.
	return bytes
}

type Bls12381Scalar struct{}

// Static interface assertion
var _ EcScalar = (*Bls12381Scalar)(nil)

func NewBls12381Scalar() *Bls12381Scalar {
	return &Bls12381Scalar{}
}

func (k Bls12381Scalar) Add(x, y *big.Int) *big.Int {
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	b := bls12381.Bls12381FqNew().SetBigInt(y)
	return a.Add(a, b).BigInt()
}

func (k Bls12381Scalar) Sub(x, y *big.Int) *big.Int {
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	b := bls12381.Bls12381FqNew().SetBigInt(y)
	return a.Sub(a, b).BigInt()
}

func (k Bls12381Scalar) Neg(x *big.Int) *big.Int {
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	return a.Neg(a).BigInt()
}

func (k Bls12381Scalar) Mul(x, y *big.Int) *big.Int {
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	b := bls12381.Bls12381FqNew().SetBigInt(y)
	return a.Mul(a, b).BigInt()
}

func (k Bls12381Scalar) Div(x, y *big.Int) *big.Int {
	c := bls12381.Bls12381FqNew()
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	b := bls12381.Bls12381FqNew().SetBigInt(y)
	_, wasInverted := c.Invert(b)
	c.Mul(a, c)
	tt := map[bool]int{false: 0, true: 1}
	return a.CMove(a, c, tt[wasInverted]).BigInt()
}

func (k Bls12381Scalar) Hash(input []byte) *big.Int {
	return new(ScalarBls12381).Hash(input).BigInt()
}

func (k Bls12381Scalar) Random() (*big.Int, error) {
	a := BLS12381G1().NewScalar().Random(crand.Reader)
	if a == nil {
		return nil, fmt.Errorf("invalid random value")
	}
	return a.BigInt(), nil
}

func (k Bls12381Scalar) Bytes(x *big.Int) []byte {
	bytes := make([]byte, 32)
	x.FillBytes(bytes) // big-endian; will left-pad.
	return bytes
}

func (k Bls12381Scalar) IsValid(x *big.Int) bool {
	a := bls12381.Bls12381FqNew().SetBigInt(x)
	return a.BigInt().Cmp(x) == 0
}

// taken from https://datatracker.ietf.org/doc/html/rfc8032
var ed25519N, _ = new(big.Int).SetString("1000000000000000000000000000000014DEF9DEA2F79CD65812631A5CF5D3ED", 16)

type Ed25519Scalar struct{}

// Static interface assertion
var _ EcScalar = (*Ed25519Scalar)(nil)

func NewEd25519Scalar() *Ed25519Scalar {
	return &Ed25519Scalar{}
}

func (k Ed25519Scalar) Add(x, y *big.Int) *big.Int {
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	b, err := internal.BigInt2Ed25519Scalar(y)
	if err != nil {
		panic(err)
	}
	a.Add(a, b)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(a.Bytes()))
}

func (k Ed25519Scalar) Sub(x, y *big.Int) *big.Int {
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	b, err := internal.BigInt2Ed25519Scalar(y)
	if err != nil {
		panic(err)
	}
	a.Subtract(a, b)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(a.Bytes()))
}

func (k Ed25519Scalar) Neg(x *big.Int) *big.Int {
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	a.Negate(a)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(a.Bytes()))
}

func (k Ed25519Scalar) Mul(x, y *big.Int) *big.Int {
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	b, err := internal.BigInt2Ed25519Scalar(y)
	if err != nil {
		panic(err)
	}
	a.Multiply(a, b)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(a.Bytes()))
}

func (k Ed25519Scalar) Div(x, y *big.Int) *big.Int {
	b, err := internal.BigInt2Ed25519Scalar(y)
	if err != nil {
		panic(err)
	}
	b.Invert(b)
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	a.Multiply(a, b)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(a.Bytes()))
}

func (k Ed25519Scalar) Hash(input []byte) *big.Int {
	v := new(ristretto.Scalar).Derive(input)
	var data [32]byte
	v.BytesInto(&data)
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(data[:]))
}

func (k Ed25519Scalar) Bytes(x *big.Int) []byte {
	a, err := internal.BigInt2Ed25519Scalar(x)
	if err != nil {
		panic(err)
	}
	return internal.ReverseScalarBytes(a.Bytes())
}

func (k Ed25519Scalar) Random() (*big.Int, error) {
	return k.RandomWithReader(crand.Reader)
}

func (k Ed25519Scalar) RandomWithReader(r io.Reader) (*big.Int, error) {
	b := make([]byte, 64)
	n, err := r.Read(b)
	if err != nil {
		return nil, err
	}
	if n != 64 {
		return nil, fmt.Errorf("insufficient bytes read")
	}
	digest := sha512.Sum512(b)
	var hBytes [32]byte
	copy(hBytes[:], digest[:])
	s, err := edwards25519.NewScalar().SetBytesWithClamping(hBytes[:])
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(internal.ReverseScalarBytes(s.Bytes())), nil
}

func (k Ed25519Scalar) IsValid(x *big.Int) bool {
	return x.Cmp(ed25519N) == -1
}
