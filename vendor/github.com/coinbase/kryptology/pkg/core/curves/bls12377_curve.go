//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

// NOTE that the bls curves are NOT constant time. There is an open issue to address it: https://github.com/coinbase/kryptology/issues/233

package curves

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"

	bls12377 "github.com/consensys/gnark-crypto/ecc/bls12-377"
	"golang.org/x/crypto/sha3"

	"github.com/coinbase/kryptology/pkg/core"
)

// See 'r' = https://eprint.iacr.org/2018/962.pdf Figure 16
var bls12377modulus = bhex("12ab655e9a2ca55660b44d1e5c37b00159aa76fed00000010a11800000000001")
var g1Inf = bls12377.G1Affine{X: [6]uint64{}, Y: [6]uint64{}}
var g2Inf = bls12377.G2Affine{
	X: bls12377.E2{A0: [6]uint64{}, A1: [6]uint64{}},
	Y: bls12377.E2{A0: [6]uint64{}, A1: [6]uint64{}},
}

type ScalarBls12377 struct {
	value *big.Int
	point Point
}

type PointBls12377G1 struct {
	value *bls12377.G1Affine
}

type PointBls12377G2 struct {
	value *bls12377.G2Affine
}

type ScalarBls12377Gt struct {
	value *bls12377.GT
}

func (s *ScalarBls12377) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarBls12377) Hash(bytes []byte) Scalar {
	xmd, err := expandMsgXmd(sha256.New(), bytes, []byte("BLS12377_XMD:SHA-256_SSWU_RO_"), 48)
	if err != nil {
		return nil
	}
	v := new(big.Int).SetBytes(xmd)
	return &ScalarBls12377{
		value: v.Mod(v, bls12377modulus),
		point: s.point,
	}
}

func (s *ScalarBls12377) Zero() Scalar {
	return &ScalarBls12377{
		value: big.NewInt(0),
		point: s.point,
	}
}

func (s *ScalarBls12377) One() Scalar {
	return &ScalarBls12377{
		value: big.NewInt(1),
		point: s.point,
	}
}

func (s *ScalarBls12377) IsZero() bool {
	return subtle.ConstantTimeCompare(s.value.Bytes(), []byte{}) == 1
}

func (s *ScalarBls12377) IsOne() bool {
	return subtle.ConstantTimeCompare(s.value.Bytes(), []byte{1}) == 1
}

func (s *ScalarBls12377) IsOdd() bool {
	return s.value.Bit(0) == 1
}

func (s *ScalarBls12377) IsEven() bool {
	return s.value.Bit(0) == 0
}

func (s *ScalarBls12377) New(value int) Scalar {
	v := big.NewInt(int64(value))
	if value < 0 {
		v.Mod(v, bls12377modulus)
	}
	return &ScalarBls12377{
		value: v,
		point: s.point,
	}
}

func (s *ScalarBls12377) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		return s.value.Cmp(r.value)
	} else {
		return -2
	}
}

func (s *ScalarBls12377) Square() Scalar {
	return &ScalarBls12377{
		value: new(big.Int).Exp(s.value, big.NewInt(2), bls12377modulus),
		point: s.point,
	}
}

func (s *ScalarBls12377) Double() Scalar {
	v := new(big.Int).Add(s.value, s.value)
	return &ScalarBls12377{
		value: v.Mod(v, bls12377modulus),
		point: s.point,
	}
}

func (s *ScalarBls12377) Invert() (Scalar, error) {
	return &ScalarBls12377{
		value: new(big.Int).ModInverse(s.value, bls12377modulus),
		point: s.point,
	}, nil
}

func (s *ScalarBls12377) Sqrt() (Scalar, error) {
	return &ScalarBls12377{
		value: new(big.Int).ModSqrt(s.value, bls12377modulus),
		point: s.point,
	}, nil
}

func (s *ScalarBls12377) Cube() Scalar {
	return &ScalarBls12377{
		value: new(big.Int).Exp(s.value, big.NewInt(3), bls12377modulus),
		point: s.point,
	}
}

func (s *ScalarBls12377) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		v := new(big.Int).Add(s.value, r.value)
		return &ScalarBls12377{
			value: v.Mod(v, bls12377modulus),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		v := new(big.Int).Sub(s.value, r.value)
		return &ScalarBls12377{
			value: v.Mod(v, bls12377modulus),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		v := new(big.Int).Mul(s.value, r.value)
		return &ScalarBls12377{
			value: v.Mod(v, bls12377modulus),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarBls12377) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		v := new(big.Int).ModInverse(r.value, bls12377modulus)
		v.Mul(v, s.value)
		return &ScalarBls12377{
			value: v.Mod(v, bls12377modulus),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377) Neg() Scalar {
	z := new(big.Int).Neg(s.value)
	return &ScalarBls12377{
		value: z.Mod(z, bls12377modulus),
		point: s.point,
	}
}

func (s *ScalarBls12377) SetBigInt(v *big.Int) (Scalar, error) {
	if v == nil {
		return nil, fmt.Errorf("invalid value")
	}
	t := new(big.Int).Mod(v, bls12377modulus)
	if t.Cmp(v) != 0 {
		return nil, fmt.Errorf("invalid value")
	}
	return &ScalarBls12377{
		value: t,
		point: s.point,
	}, nil
}

func (s *ScalarBls12377) BigInt() *big.Int {
	return new(big.Int).Set(s.value)
}

func (s *ScalarBls12377) Bytes() []byte {
	var out [32]byte
	return s.value.FillBytes(out[:])
}

func (s *ScalarBls12377) SetBytes(bytes []byte) (Scalar, error) {
	value := new(big.Int).SetBytes(bytes)
	t := new(big.Int).Mod(value, bls12377modulus)
	if t.Cmp(value) != 0 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	return &ScalarBls12377{
		value: t,
		point: s.point,
	}, nil
}

func (s *ScalarBls12377) SetBytesWide(bytes []byte) (Scalar, error) {
	if len(bytes) < 32 || len(bytes) > 128 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	value := new(big.Int).SetBytes(bytes)
	t := new(big.Int).Mod(value, bls12377modulus)
	return &ScalarBls12377{
		value: t,
		point: s.point,
	}, nil
}

func (s *ScalarBls12377) Point() Point {
	return s.point.Identity()
}

func (s *ScalarBls12377) Clone() Scalar {
	return &ScalarBls12377{
		value: new(big.Int).Set(s.value),
		point: s.point,
	}
}

func (s *ScalarBls12377) SetPoint(p Point) PairingScalar {
	return &ScalarBls12377{
		value: new(big.Int).Set(s.value),
		point: p,
	}
}

func (s *ScalarBls12377) Order() *big.Int {
	return bls12377modulus
}

func (s *ScalarBls12377) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarBls12377) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12377)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	s.point = ss.point
	return nil
}

func (s *ScalarBls12377) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarBls12377) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12377)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	s.point = ss.point
	return nil
}

func (s *ScalarBls12377) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarBls12377) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarBls12377)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

func (p *PointBls12377G1) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointBls12377G1) Hash(bytes []byte) Point {
	var domain = []byte("BLS12377G1_XMD:SHA-256_SVDW_RO_")
	pt, err := bls12377.HashToCurveG1Svdw(bytes, domain)
	if err != nil {
		return nil
	}
	return &PointBls12377G1{value: &pt}
}

func (p *PointBls12377G1) Identity() Point {
	t := bls12377.G1Affine{}
	return &PointBls12377G1{
		value: t.Set(&g1Inf),
	}
}

func (p *PointBls12377G1) Generator() Point {
	t := bls12377.G1Affine{}
	_, _, g1Aff, _ := bls12377.Generators()
	return &PointBls12377G1{
		value: t.Set(&g1Aff),
	}
}

func (p *PointBls12377G1) IsIdentity() bool {
	return p.value.IsInfinity()
}

func (p *PointBls12377G1) IsNegative() bool {
	// According to https://github.com/zcash/librustzcash/blob/6e0364cd42a2b3d2b958a54771ef51a8db79dd29/pairing/src/bls12_381/README.md#serialization
	// This bit represents the sign of the `y` coordinate which is what we want

	return (p.value.Bytes()[0]>>5)&1 == 1
}

func (p *PointBls12377G1) IsOnCurve() bool {
	return p.value.IsOnCurve()
}

func (p *PointBls12377G1) Double() Point {
	t := &bls12377.G1Jac{}
	t.FromAffine(p.value)
	t.DoubleAssign()
	value := bls12377.G1Affine{}
	return &PointBls12377G1{value.FromJacobian(t)}
}

func (p *PointBls12377G1) Scalar() Scalar {
	return &ScalarBls12377{
		value: new(big.Int),
		point: new(PointBls12377G1),
	}
}

func (p *PointBls12377G1) Neg() Point {
	value := &bls12377.G1Affine{}
	value.Neg(p.value)
	return &PointBls12377G1{value}
}

func (p *PointBls12377G1) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12377G1)
	if ok {
		value := &bls12377.G1Affine{}
		return &PointBls12377G1{value.Add(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G1) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12377G1)
	if ok {
		value := &bls12377.G1Affine{}
		return &PointBls12377G1{value.Sub(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G1) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		value := &bls12377.G1Affine{}
		return &PointBls12377G1{value.ScalarMultiplication(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G1) Equal(rhs Point) bool {
	r, ok := rhs.(*PointBls12377G1)
	if ok {
		return p.value.Equal(r.value)
	} else {
		return false
	}
}

func (p *PointBls12377G1) Set(x, y *big.Int) (Point, error) {
	if x.Cmp(core.Zero) == 0 &&
		y.Cmp(core.Zero) == 0 {
		return p.Identity(), nil
	}
	var data [96]byte
	x.FillBytes(data[:48])
	y.FillBytes(data[48:])
	value := &bls12377.G1Affine{}
	_, err := value.SetBytes(data[:])
	if err != nil {
		return nil, fmt.Errorf("invalid coordinates")
	}
	return &PointBls12377G1{value}, nil
}

func (p *PointBls12377G1) ToAffineCompressed() []byte {
	v := p.value.Bytes()
	return v[:]
}

func (p *PointBls12377G1) ToAffineUncompressed() []byte {
	v := p.value.RawBytes()
	return v[:]
}

func (p *PointBls12377G1) FromAffineCompressed(bytes []byte) (Point, error) {
	if len(bytes) != bls12377.SizeOfG1AffineCompressed {
		return nil, fmt.Errorf("invalid point")
	}
	value := &bls12377.G1Affine{}
	_, err := value.SetBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &PointBls12377G1{value}, nil
}

func (p *PointBls12377G1) FromAffineUncompressed(bytes []byte) (Point, error) {
	if len(bytes) != bls12377.SizeOfG1AffineUncompressed {
		return nil, fmt.Errorf("invalid point")
	}
	value := &bls12377.G1Affine{}
	_, err := value.SetBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &PointBls12377G1{value}, nil
}

func (p *PointBls12377G1) CurveName() string {
	return "BLS12377G1"
}

func (p *PointBls12377G1) SumOfProducts(points []Point, scalars []Scalar) Point {
	nScalars := make([]*big.Int, len(scalars))
	for i, sc := range scalars {
		s, ok := sc.(*ScalarBls12377)
		if !ok {
			return nil
		}
		nScalars[i] = s.value
	}
	return sumOfProductsPippenger(points, nScalars)
}

func (p *PointBls12377G1) OtherGroup() PairingPoint {
	return new(PointBls12377G2).Identity().(PairingPoint)
}

func (p *PointBls12377G1) Pairing(rhs PairingPoint) Scalar {
	pt, ok := rhs.(*PointBls12377G2)
	if !ok {
		return nil
	}
	if !p.value.IsInSubGroup() ||
		!pt.value.IsInSubGroup() {
		return nil
	}
	value := bls12377.GT{}
	if p.value.IsInfinity() || pt.value.IsInfinity() {
		return &ScalarBls12377Gt{&value}
	}
	value, err := bls12377.Pair([]bls12377.G1Affine{*p.value}, []bls12377.G2Affine{*pt.value})
	if err != nil {
		return nil
	}

	return &ScalarBls12377Gt{&value}
}

func (p *PointBls12377G1) MultiPairing(points ...PairingPoint) Scalar {
	return multiPairingBls12377(points...)
}

func (p *PointBls12377G1) X() *big.Int {
	b := p.value.RawBytes()
	return new(big.Int).SetBytes(b[:48])
}

func (p *PointBls12377G1) Y() *big.Int {
	b := p.value.RawBytes()
	return new(big.Int).SetBytes(b[48:])
}

func (p *PointBls12377G1) Modulus() *big.Int {
	return bls12377modulus
}

func (p *PointBls12377G1) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointBls12377G1) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12377G1)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointBls12377G1) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointBls12377G1) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12377G1)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointBls12377G1) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointBls12377G1) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointBls12377G1)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}

func (p *PointBls12377G2) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointBls12377G2) Hash(bytes []byte) Point {
	var domain = []byte("BLS12377G2_XMD:SHA-256_SVDW_RO_")
	pt, err := bls12377.HashToCurveG2Svdw(bytes, domain)
	if err != nil {
		return nil
	}
	return &PointBls12377G2{value: &pt}
}

func (p *PointBls12377G2) Identity() Point {
	t := bls12377.G2Affine{}
	return &PointBls12377G2{
		value: t.Set(&g2Inf),
	}
}

func (p *PointBls12377G2) Generator() Point {
	t := bls12377.G2Affine{}
	_, _, _, g2Aff := bls12377.Generators()
	return &PointBls12377G2{
		value: t.Set(&g2Aff),
	}
}

func (p *PointBls12377G2) IsIdentity() bool {
	return p.value.IsInfinity()
}

func (p *PointBls12377G2) IsNegative() bool {
	// According to https://github.com/zcash/librustzcash/blob/6e0364cd42a2b3d2b958a54771ef51a8db79dd29/pairing/src/bls12_381/README.md#serialization
	// This bit represents the sign of the `y` coordinate which is what we want
	return (p.value.Bytes()[0]>>5)&1 == 1
}

func (p *PointBls12377G2) IsOnCurve() bool {
	return p.value.IsOnCurve()
}

func (p *PointBls12377G2) Double() Point {
	t := &bls12377.G2Jac{}
	t.FromAffine(p.value)
	t.DoubleAssign()
	value := bls12377.G2Affine{}
	return &PointBls12377G2{value.FromJacobian(t)}
}

func (p *PointBls12377G2) Scalar() Scalar {
	return &ScalarBls12377{
		value: new(big.Int),
		point: new(PointBls12377G2),
	}
}

func (p *PointBls12377G2) Neg() Point {
	value := &bls12377.G2Affine{}
	value.Neg(p.value)
	return &PointBls12377G2{value}
}

func (p *PointBls12377G2) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12377G2)
	if ok {
		value := &bls12377.G2Affine{}
		return &PointBls12377G2{value.Add(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G2) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12377G2)
	if ok {
		value := &bls12377.G2Affine{}
		return &PointBls12377G2{value.Sub(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G2) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarBls12377)
	if ok {
		value := &bls12377.G2Affine{}
		return &PointBls12377G2{value.ScalarMultiplication(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointBls12377G2) Equal(rhs Point) bool {
	r, ok := rhs.(*PointBls12377G2)
	if ok {
		return p.value.Equal(r.value)
	} else {
		return false
	}
}

func (p *PointBls12377G2) Set(x, y *big.Int) (Point, error) {
	if x.Cmp(core.Zero) == 0 &&
		y.Cmp(core.Zero) == 0 {
		return p.Identity(), nil
	}
	var data [192]byte
	x.FillBytes(data[:96])
	y.FillBytes(data[96:])
	value := &bls12377.G2Affine{}
	_, err := value.SetBytes(data[:])
	if err != nil {
		return nil, fmt.Errorf("invalid coordinates")
	}
	return &PointBls12377G2{value}, nil
}

func (p *PointBls12377G2) ToAffineCompressed() []byte {
	v := p.value.Bytes()
	return v[:]
}

func (p *PointBls12377G2) ToAffineUncompressed() []byte {
	v := p.value.RawBytes()
	return v[:]
}

func (p *PointBls12377G2) FromAffineCompressed(bytes []byte) (Point, error) {
	if len(bytes) != bls12377.SizeOfG2AffineCompressed {
		return nil, fmt.Errorf("invalid point")
	}
	value := &bls12377.G2Affine{}
	_, err := value.SetBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &PointBls12377G2{value}, nil
}

func (p *PointBls12377G2) FromAffineUncompressed(bytes []byte) (Point, error) {
	if len(bytes) != bls12377.SizeOfG2AffineUncompressed {
		return nil, fmt.Errorf("invalid point")
	}
	value := &bls12377.G2Affine{}
	_, err := value.SetBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &PointBls12377G2{value}, nil
}

func (p *PointBls12377G2) CurveName() string {
	return "BLS12377G2"
}

func (p *PointBls12377G2) SumOfProducts(points []Point, scalars []Scalar) Point {
	nScalars := make([]*big.Int, len(scalars))
	for i, sc := range scalars {
		s, ok := sc.(*ScalarBls12377)
		if !ok {
			return nil
		}
		nScalars[i] = s.value
	}
	return sumOfProductsPippenger(points, nScalars)
}

func (p *PointBls12377G2) OtherGroup() PairingPoint {
	return new(PointBls12377G1).Identity().(PairingPoint)
}

func (p *PointBls12377G2) Pairing(rhs PairingPoint) Scalar {
	pt, ok := rhs.(*PointBls12377G1)
	if !ok {
		return nil
	}
	if !p.value.IsInSubGroup() ||
		!pt.value.IsInSubGroup() {
		return nil
	}
	value := bls12377.GT{}
	if p.value.IsInfinity() || pt.value.IsInfinity() {
		return &ScalarBls12377Gt{&value}
	}
	value, err := bls12377.Pair([]bls12377.G1Affine{*pt.value}, []bls12377.G2Affine{*p.value})
	if err != nil {
		return nil
	}

	return &ScalarBls12377Gt{&value}
}

func (p *PointBls12377G2) MultiPairing(points ...PairingPoint) Scalar {
	return multiPairingBls12377(points...)
}

func (p *PointBls12377G2) X() *big.Int {
	b := p.value.RawBytes()
	return new(big.Int).SetBytes(b[:96])
}

func (p *PointBls12377G2) Y() *big.Int {
	b := p.value.RawBytes()
	return new(big.Int).SetBytes(b[96:])
}

func (p *PointBls12377G2) Modulus() *big.Int {
	return bls12377modulus
}

func (p *PointBls12377G2) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointBls12377G2) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12377G2)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointBls12377G2) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointBls12377G2) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12377G2)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointBls12377G2) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointBls12377G2) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointBls12377G2)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}

func multiPairingBls12377(points ...PairingPoint) Scalar {
	if len(points)%2 != 0 {
		return nil
	}
	g1Arr := make([]bls12377.G1Affine, 0, len(points)/2)
	g2Arr := make([]bls12377.G2Affine, 0, len(points)/2)
	valid := true
	for i := 0; i < len(points); i += 2 {
		pt1, ok := points[i].(*PointBls12377G1)
		valid = valid && ok
		pt2, ok := points[i+1].(*PointBls12377G2)
		valid = valid && ok
		if valid {
			valid = valid && pt1.value.IsInSubGroup()
			valid = valid && pt2.value.IsInSubGroup()
		}
		if valid {
			g1Arr = append(g1Arr, *pt1.value)
			g2Arr = append(g2Arr, *pt2.value)
		}
	}
	if !valid {
		return nil
	}

	value, err := bls12377.Pair(g1Arr, g2Arr)
	if err != nil {
		return nil
	}

	return &ScalarBls12377Gt{&value}
}

func (s *ScalarBls12377Gt) Random(reader io.Reader) Scalar {
	const width = 48
	offset := 0
	var data [bls12377.SizeOfGT]byte
	for i := 0; i < 12; i++ {
		tv, err := rand.Int(reader, bls12377modulus)
		if err != nil {
			return nil
		}
		tv.FillBytes(data[offset*width : (offset+1)*width])
		offset++
	}
	value := bls12377.GT{}
	err := value.SetBytes(data[:])
	if err != nil {
		return nil
	}
	return &ScalarBls12377Gt{&value}
}

func (s *ScalarBls12377Gt) Hash(bytes []byte) Scalar {
	reader := sha3.NewShake256()
	n, err := reader.Write(bytes)
	if err != nil {
		return nil
	}
	if n != len(bytes) {
		return nil
	}
	return s.Random(reader)
}

func (s *ScalarBls12377Gt) Zero() Scalar {
	var t [bls12377.SizeOfGT]byte
	value := bls12377.GT{}
	err := value.SetBytes(t[:])
	if err != nil {
		return nil
	}
	return &ScalarBls12377Gt{&value}
}

func (s *ScalarBls12377Gt) One() Scalar {
	value := bls12377.GT{}
	return &ScalarBls12377Gt{value.SetOne()}
}

func (s *ScalarBls12377Gt) IsZero() bool {
	r := byte(0)
	b := s.value.Bytes()
	for _, i := range b {
		r |= i
	}
	return r == 0
}

func (s *ScalarBls12377Gt) IsOne() bool {
	o := bls12377.GT{}
	return s.value.Equal(o.SetOne())
}

func (s *ScalarBls12377Gt) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarBls12377Gt) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12377Gt)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarBls12377Gt) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarBls12377Gt) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12377Gt)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarBls12377Gt) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarBls12377Gt) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarBls12377Gt)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

func (s *ScalarBls12377Gt) IsOdd() bool {
	data := s.value.Bytes()
	return data[len(data)-1]&1 == 1
}

func (s *ScalarBls12377Gt) IsEven() bool {
	data := s.value.Bytes()
	return data[len(data)-1]&1 == 0
}

func (s *ScalarBls12377Gt) New(input int) Scalar {
	var data [576]byte
	data[3] = byte(input >> 24 & 0xFF)
	data[2] = byte(input >> 16 & 0xFF)
	data[1] = byte(input >> 8 & 0xFF)
	data[0] = byte(input & 0xFF)

	value := bls12377.GT{}
	err := value.SetBytes(data[:])
	if err != nil {
		return nil
	}
	return &ScalarBls12377Gt{&value}
}

func (s *ScalarBls12377Gt) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarBls12377Gt)
	if ok && s.value.Equal(r.value) {
		return 0
	} else {
		return -2
	}
}

func (s *ScalarBls12377Gt) Square() Scalar {
	value := bls12377.GT{}
	return &ScalarBls12377Gt{
		value.Square(s.value),
	}
}

func (s *ScalarBls12377Gt) Double() Scalar {
	value := &bls12377.GT{}
	return &ScalarBls12377Gt{
		value.Add(s.value, s.value),
	}
}

func (s *ScalarBls12377Gt) Invert() (Scalar, error) {
	value := &bls12377.GT{}
	return &ScalarBls12377Gt{
		value.Inverse(s.value),
	}, nil
}

func (s *ScalarBls12377Gt) Sqrt() (Scalar, error) {
	// Not implemented
	return nil, nil
}

func (s *ScalarBls12377Gt) Cube() Scalar {
	value := &bls12377.GT{}
	value.Square(s.value)
	value.Mul(value, s.value)
	return &ScalarBls12377Gt{
		value,
	}
}

func (s *ScalarBls12377Gt) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377Gt)
	if ok {
		value := &bls12377.GT{}
		return &ScalarBls12377Gt{
			value.Add(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377Gt) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377Gt)
	if ok {
		value := &bls12377.GT{}
		return &ScalarBls12377Gt{
			value.Sub(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377Gt) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377Gt)
	if ok {
		value := &bls12377.GT{}
		return &ScalarBls12377Gt{
			value.Mul(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377Gt) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarBls12377Gt) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12377Gt)
	if ok {
		value := &bls12377.GT{}
		value.Inverse(r.value)
		value.Mul(value, s.value)
		return &ScalarBls12377Gt{
			value,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12377Gt) Neg() Scalar {
	sValue := &bls12377.GT{}
	sValue.SetOne()
	value := &bls12377.GT{}
	value.SetOne()
	value.Sub(value, sValue)
	return &ScalarBls12377Gt{
		value.Sub(value, s.value),
	}
}

func (s *ScalarBls12377Gt) SetBigInt(v *big.Int) (Scalar, error) {
	var bytes [576]byte
	v.FillBytes(bytes[:])
	return s.SetBytes(bytes[:])
}

func (s *ScalarBls12377Gt) BigInt() *big.Int {
	b := s.value.Bytes()
	return new(big.Int).SetBytes(b[:])
}

func (s *ScalarBls12377Gt) Point() Point {
	p := &PointBls12377G1{}
	return p.Identity()
}

func (s *ScalarBls12377Gt) Bytes() []byte {
	b := s.value.Bytes()
	return b[:]
}

func (s *ScalarBls12377Gt) SetBytes(bytes []byte) (Scalar, error) {
	value := &bls12377.GT{}
	err := value.SetBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &ScalarBls12377Gt{value}, nil
}

func (s *ScalarBls12377Gt) SetBytesWide(bytes []byte) (Scalar, error) {
	l := len(bytes)
	if l != 1152 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	value := &bls12377.GT{}
	err := value.SetBytes(bytes[:l/2])
	if err != nil {
		return nil, err
	}
	value2 := &bls12377.GT{}
	err = value2.SetBytes(bytes[l/2:])
	if err != nil {
		return nil, err
	}
	value.Add(value, value2)
	return &ScalarBls12377Gt{value}, nil
}

func (s *ScalarBls12377Gt) Clone() Scalar {
	value := &bls12377.GT{}
	return &ScalarBls12377Gt{
		value.Set(s.value),
	}
}
