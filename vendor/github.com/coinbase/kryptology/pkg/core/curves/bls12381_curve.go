//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/sha3"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	"github.com/coinbase/kryptology/pkg/core/curves/native/bls12381"
)

var bls12381modulus = bhex("1a0111ea397fe69a4b1ba7b6434bacd764774b84f38512bf6730d2a0f6b0f6241eabfffeb153ffffb9feffffffffaaab")

type ScalarBls12381 struct {
	Value *native.Field
	point Point
}

type PointBls12381G1 struct {
	Value *bls12381.G1
}

type PointBls12381G2 struct {
	Value *bls12381.G2
}

type ScalarBls12381Gt struct {
	Value *bls12381.Gt
}

func (s *ScalarBls12381) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarBls12381) Hash(bytes []byte) Scalar {
	dst := []byte("BLS12381_XMD:SHA-256_SSWU_RO_")
	xmd := native.ExpandMsgXmd(native.EllipticPointHasherSha256(), bytes, dst, 48)
	var t [64]byte
	copy(t[:48], internal.ReverseScalarBytes(xmd))

	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().SetBytesWide(&t),
		point: s.point,
	}
}

func (s *ScalarBls12381) Zero() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().SetZero(),
		point: s.point,
	}
}

func (s *ScalarBls12381) One() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().SetOne(),
		point: s.point,
	}
}

func (s *ScalarBls12381) IsZero() bool {
	return s.Value.IsZero() == 1
}

func (s *ScalarBls12381) IsOne() bool {
	return s.Value.IsOne() == 1
}

func (s *ScalarBls12381) IsOdd() bool {
	bytes := s.Value.Bytes()
	return bytes[0]&1 == 1
}

func (s *ScalarBls12381) IsEven() bool {
	bytes := s.Value.Bytes()
	return bytes[0]&1 == 0
}

func (s *ScalarBls12381) New(value int) Scalar {
	t := bls12381.Bls12381FqNew()
	v := big.NewInt(int64(value))
	if value < 0 {
		v.Mod(v, t.Params.BiModulus)
	}
	return &ScalarBls12381{
		Value: t.SetBigInt(v),
		point: s.point,
	}
}

func (s *ScalarBls12381) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return s.Value.Cmp(r.Value)
	} else {
		return -2
	}
}

func (s *ScalarBls12381) Square() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().Square(s.Value),
		point: s.point,
	}
}

func (s *ScalarBls12381) Double() Scalar {
	v := bls12381.Bls12381FqNew().Double(s.Value)
	return &ScalarBls12381{
		Value: v,
		point: s.point,
	}
}

func (s *ScalarBls12381) Invert() (Scalar, error) {
	value, wasInverted := bls12381.Bls12381FqNew().Invert(s.Value)
	if !wasInverted {
		return nil, fmt.Errorf("inverse doesn't exist")
	}
	return &ScalarBls12381{
		Value: value,
		point: s.point,
	}, nil
}

func (s *ScalarBls12381) Sqrt() (Scalar, error) {
	value, wasSquare := bls12381.Bls12381FqNew().Sqrt(s.Value)
	if !wasSquare {
		return nil, fmt.Errorf("not a square")
	}
	return &ScalarBls12381{
		Value: value,
		point: s.point,
	}, nil
}

func (s *ScalarBls12381) Cube() Scalar {
	value := bls12381.Bls12381FqNew().Square(s.Value)
	value.Mul(value, s.Value)
	return &ScalarBls12381{
		Value: value,
		point: s.point,
	}
}

func (s *ScalarBls12381) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return &ScalarBls12381{
			Value: bls12381.Bls12381FqNew().Add(s.Value, r.Value),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return &ScalarBls12381{
			Value: bls12381.Bls12381FqNew().Sub(s.Value, r.Value),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return &ScalarBls12381{
			Value: bls12381.Bls12381FqNew().Mul(s.Value, r.Value),
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarBls12381) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		v, wasInverted := bls12381.Bls12381FqNew().Invert(r.Value)
		if !wasInverted {
			return nil
		}
		v.Mul(v, s.Value)
		return &ScalarBls12381{
			Value: v,
			point: s.point,
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381) Neg() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().Neg(s.Value),
		point: s.point,
	}
}

func (s *ScalarBls12381) SetBigInt(v *big.Int) (Scalar, error) {
	if v == nil {
		return nil, fmt.Errorf("invalid value")
	}
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().SetBigInt(v),
		point: s.point,
	}, nil
}

func (s *ScalarBls12381) BigInt() *big.Int {
	return s.Value.BigInt()
}

func (s *ScalarBls12381) Bytes() []byte {
	t := s.Value.Bytes()
	return internal.ReverseScalarBytes(t[:])
}

func (s *ScalarBls12381) SetBytes(bytes []byte) (Scalar, error) {
	if len(bytes) != 32 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [32]byte
	copy(seq[:], internal.ReverseScalarBytes(bytes))
	value, err := bls12381.Bls12381FqNew().SetBytes(&seq)
	if err != nil {
		return nil, err
	}
	return &ScalarBls12381{
		value, s.point,
	}, nil
}

func (s *ScalarBls12381) SetBytesWide(bytes []byte) (Scalar, error) {
	if len(bytes) != 64 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [64]byte
	copy(seq[:], bytes)
	return &ScalarBls12381{
		bls12381.Bls12381FqNew().SetBytesWide(&seq), s.point,
	}, nil
}

func (s *ScalarBls12381) Point() Point {
	return s.point.Identity()
}

func (s *ScalarBls12381) Clone() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().Set(s.Value),
		point: s.point,
	}
}

func (s *ScalarBls12381) SetPoint(p Point) PairingScalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew().Set(s.Value),
		point: p,
	}
}

func (s *ScalarBls12381) Order() *big.Int {
	return s.Value.Params.BiModulus
}

func (s *ScalarBls12381) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarBls12381) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12381)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.Value = ss.Value
	s.point = ss.point
	return nil
}

func (s *ScalarBls12381) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarBls12381) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12381)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.Value = ss.Value
	s.point = ss.point
	return nil
}

func (s *ScalarBls12381) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarBls12381) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarBls12381)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.Value = S.Value
	return nil
}

func (p *PointBls12381G1) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointBls12381G1) Hash(bytes []byte) Point {
	var domain = []byte("BLS12381G1_XMD:SHA-256_SSWU_RO_")
	pt := new(bls12381.G1).Hash(native.EllipticPointHasherSha256(), bytes, domain)
	return &PointBls12381G1{Value: pt}
}

func (p *PointBls12381G1) Identity() Point {
	return &PointBls12381G1{
		Value: new(bls12381.G1).Identity(),
	}
}

func (p *PointBls12381G1) Generator() Point {
	return &PointBls12381G1{
		Value: new(bls12381.G1).Generator(),
	}
}

func (p *PointBls12381G1) IsIdentity() bool {
	return p.Value.IsIdentity() == 1
}

func (p *PointBls12381G1) IsNegative() bool {
	// According to https://github.com/zcash/librustzcash/blob/6e0364cd42a2b3d2b958a54771ef51a8db79dd29/pairing/src/bls12_381/README.md#serialization
	// This bit represents the sign of the `y` coordinate which is what we want
	return (p.Value.ToCompressed()[0]>>5)&1 == 1
}

func (p *PointBls12381G1) IsOnCurve() bool {
	return p.Value.IsOnCurve() == 1
}

func (p *PointBls12381G1) Double() Point {
	return &PointBls12381G1{new(bls12381.G1).Double(p.Value)}
}

func (p *PointBls12381G1) Scalar() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew(),
		point: new(PointBls12381G1),
	}
}

func (p *PointBls12381G1) Neg() Point {
	return &PointBls12381G1{new(bls12381.G1).Neg(p.Value)}
}

func (p *PointBls12381G1) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12381G1)
	if ok {
		return &PointBls12381G1{new(bls12381.G1).Add(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G1) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12381G1)
	if ok {
		return &PointBls12381G1{new(bls12381.G1).Sub(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G1) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return &PointBls12381G1{new(bls12381.G1).Mul(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G1) Equal(rhs Point) bool {
	r, ok := rhs.(*PointBls12381G1)
	if ok {
		return p.Value.Equal(r.Value) == 1
	} else {
		return false
	}
}

func (p *PointBls12381G1) Set(x, y *big.Int) (Point, error) {
	value, err := new(bls12381.G1).SetBigInt(x, y)
	if err != nil {
		return nil, fmt.Errorf("invalid coordinates")
	}
	return &PointBls12381G1{value}, nil
}

func (p *PointBls12381G1) ToAffineCompressed() []byte {
	out := p.Value.ToCompressed()
	return out[:]
}

func (p *PointBls12381G1) ToAffineUncompressed() []byte {
	out := p.Value.ToUncompressed()
	return out[:]
}

func (p *PointBls12381G1) FromAffineCompressed(bytes []byte) (Point, error) {
	var b [bls12381.FieldBytes]byte
	copy(b[:], bytes)
	value, err := new(bls12381.G1).FromCompressed(&b)
	if err != nil {
		return nil, err
	}
	return &PointBls12381G1{value}, nil
}

func (p *PointBls12381G1) FromAffineUncompressed(bytes []byte) (Point, error) {
	var b [96]byte
	copy(b[:], bytes)
	value, err := new(bls12381.G1).FromUncompressed(&b)
	if err != nil {
		return nil, err
	}
	return &PointBls12381G1{value}, nil
}

func (p *PointBls12381G1) CurveName() string {
	return "BLS12381G1"
}

func (p *PointBls12381G1) SumOfProducts(points []Point, scalars []Scalar) Point {
	nPoints := make([]*bls12381.G1, len(points))
	nScalars := make([]*native.Field, len(scalars))
	for i, pt := range points {
		pp, ok := pt.(*PointBls12381G1)
		if !ok {
			return nil
		}
		nPoints[i] = pp.Value
	}
	for i, sc := range scalars {
		s, ok := sc.(*ScalarBls12381)
		if !ok {
			return nil
		}
		nScalars[i] = s.Value
	}
	value, err := new(bls12381.G1).SumOfProducts(nPoints, nScalars)
	if err != nil {
		return nil
	}
	return &PointBls12381G1{value}
}

func (p *PointBls12381G1) OtherGroup() PairingPoint {
	return new(PointBls12381G2).Identity().(PairingPoint)
}

func (p *PointBls12381G1) Pairing(rhs PairingPoint) Scalar {
	pt, ok := rhs.(*PointBls12381G2)
	if !ok {
		return nil
	}
	e := new(bls12381.Engine)
	e.AddPair(p.Value, pt.Value)

	value := e.Result()

	return &ScalarBls12381Gt{value}
}

func (p *PointBls12381G1) MultiPairing(points ...PairingPoint) Scalar {
	return multiPairing(points...)
}

func (p *PointBls12381G1) X() *big.Int {
	return p.Value.GetX().BigInt()
}

func (p *PointBls12381G1) Y() *big.Int {
	return p.Value.GetY().BigInt()
}

func (p *PointBls12381G1) Modulus() *big.Int {
	return bls12381modulus
}

func (p *PointBls12381G1) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointBls12381G1) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12381G1)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.Value = ppt.Value
	return nil
}

func (p *PointBls12381G1) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointBls12381G1) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12381G1)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.Value = ppt.Value
	return nil
}

func (p *PointBls12381G1) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointBls12381G1) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointBls12381G1)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.Value = P.Value
	return nil
}

func (p *PointBls12381G2) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointBls12381G2) Hash(bytes []byte) Point {
	var domain = []byte("BLS12381G2_XMD:SHA-256_SSWU_RO_")
	pt := new(bls12381.G2).Hash(native.EllipticPointHasherSha256(), bytes, domain)
	return &PointBls12381G2{Value: pt}
}

func (p *PointBls12381G2) Identity() Point {
	return &PointBls12381G2{
		Value: new(bls12381.G2).Identity(),
	}
}

func (p *PointBls12381G2) Generator() Point {
	return &PointBls12381G2{
		Value: new(bls12381.G2).Generator(),
	}
}

func (p *PointBls12381G2) IsIdentity() bool {
	return p.Value.IsIdentity() == 1
}

func (p *PointBls12381G2) IsNegative() bool {
	// According to https://github.com/zcash/librustzcash/blob/6e0364cd42a2b3d2b958a54771ef51a8db79dd29/pairing/src/bls12_381/README.md#serialization
	// This bit represents the sign of the `y` coordinate which is what we want
	return (p.Value.ToCompressed()[0]>>5)&1 == 1
}

func (p *PointBls12381G2) IsOnCurve() bool {
	return p.Value.IsOnCurve() == 1
}

func (p *PointBls12381G2) Double() Point {
	return &PointBls12381G2{new(bls12381.G2).Double(p.Value)}
}

func (p *PointBls12381G2) Scalar() Scalar {
	return &ScalarBls12381{
		Value: bls12381.Bls12381FqNew(),
		point: new(PointBls12381G2),
	}
}

func (p *PointBls12381G2) Neg() Point {
	return &PointBls12381G2{new(bls12381.G2).Neg(p.Value)}
}

func (p *PointBls12381G2) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12381G2)
	if ok {
		return &PointBls12381G2{new(bls12381.G2).Add(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G2) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointBls12381G2)
	if ok {
		return &PointBls12381G2{new(bls12381.G2).Sub(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G2) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarBls12381)
	if ok {
		return &PointBls12381G2{new(bls12381.G2).Mul(p.Value, r.Value)}
	} else {
		return nil
	}
}

func (p *PointBls12381G2) Equal(rhs Point) bool {
	r, ok := rhs.(*PointBls12381G2)
	if ok {
		return p.Value.Equal(r.Value) == 1
	} else {
		return false
	}
}

func (p *PointBls12381G2) Set(x, y *big.Int) (Point, error) {
	value, err := new(bls12381.G2).SetBigInt(x, y)
	if err != nil {
		return nil, fmt.Errorf("invalid coordinates")
	}
	return &PointBls12381G2{value}, nil
}

func (p *PointBls12381G2) ToAffineCompressed() []byte {
	out := p.Value.ToCompressed()
	return out[:]
}

func (p *PointBls12381G2) ToAffineUncompressed() []byte {
	out := p.Value.ToUncompressed()
	return out[:]
}

func (p *PointBls12381G2) FromAffineCompressed(bytes []byte) (Point, error) {
	var b [bls12381.WideFieldBytes]byte
	copy(b[:], bytes)
	value, err := new(bls12381.G2).FromCompressed(&b)
	if err != nil {
		return nil, err
	}
	return &PointBls12381G2{value}, nil
}

func (p *PointBls12381G2) FromAffineUncompressed(bytes []byte) (Point, error) {
	var b [bls12381.DoubleWideFieldBytes]byte
	copy(b[:], bytes)
	value, err := new(bls12381.G2).FromUncompressed(&b)
	if err != nil {
		return nil, err
	}
	return &PointBls12381G2{value}, nil
}

func (p *PointBls12381G2) CurveName() string {
	return "BLS12381G2"
}

func (p *PointBls12381G2) SumOfProducts(points []Point, scalars []Scalar) Point {
	nPoints := make([]*bls12381.G2, len(points))
	nScalars := make([]*native.Field, len(scalars))
	for i, pt := range points {
		pp, ok := pt.(*PointBls12381G2)
		if !ok {
			return nil
		}
		nPoints[i] = pp.Value
	}
	for i, sc := range scalars {
		s, ok := sc.(*ScalarBls12381)
		if !ok {
			return nil
		}
		nScalars[i] = s.Value
	}
	value, err := new(bls12381.G2).SumOfProducts(nPoints, nScalars)
	if err != nil {
		return nil
	}
	return &PointBls12381G2{value}
}

func (p *PointBls12381G2) OtherGroup() PairingPoint {
	return new(PointBls12381G1).Identity().(PairingPoint)
}

func (p *PointBls12381G2) Pairing(rhs PairingPoint) Scalar {
	pt, ok := rhs.(*PointBls12381G1)
	if !ok {
		return nil
	}
	e := new(bls12381.Engine)
	e.AddPair(pt.Value, p.Value)

	value := e.Result()

	return &ScalarBls12381Gt{value}
}

func (p *PointBls12381G2) MultiPairing(points ...PairingPoint) Scalar {
	return multiPairing(points...)
}

func (p *PointBls12381G2) X() *big.Int {
	x := p.Value.ToUncompressed()
	return new(big.Int).SetBytes(x[:bls12381.WideFieldBytes])
}

func (p *PointBls12381G2) Y() *big.Int {
	y := p.Value.ToUncompressed()
	return new(big.Int).SetBytes(y[bls12381.WideFieldBytes:])
}

func (p *PointBls12381G2) Modulus() *big.Int {
	return bls12381modulus
}

func (p *PointBls12381G2) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointBls12381G2) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12381G2)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.Value = ppt.Value
	return nil
}

func (p *PointBls12381G2) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointBls12381G2) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointBls12381G2)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.Value = ppt.Value
	return nil
}

func (p *PointBls12381G2) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointBls12381G2) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointBls12381G2)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.Value = P.Value
	return nil
}

func multiPairing(points ...PairingPoint) Scalar {
	if len(points)%2 != 0 {
		return nil
	}
	valid := true
	eng := new(bls12381.Engine)
	for i := 0; i < len(points); i += 2 {
		pt1, ok := points[i].(*PointBls12381G1)
		valid = valid && ok
		pt2, ok := points[i+1].(*PointBls12381G2)
		valid = valid && ok
		if valid {
			eng.AddPair(pt1.Value, pt2.Value)
		}
	}
	if !valid {
		return nil
	}

	value := eng.Result()
	return &ScalarBls12381Gt{value}
}

func (s *ScalarBls12381Gt) Random(reader io.Reader) Scalar {
	value, err := new(bls12381.Gt).Random(reader)
	if err != nil {
		return nil
	}
	return &ScalarBls12381Gt{value}
}

func (s *ScalarBls12381Gt) Hash(bytes []byte) Scalar {
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

func (s *ScalarBls12381Gt) Zero() Scalar {
	return &ScalarBls12381Gt{new(bls12381.Gt)}
}

func (s *ScalarBls12381Gt) One() Scalar {
	return &ScalarBls12381Gt{new(bls12381.Gt).SetOne()}
}

func (s *ScalarBls12381Gt) IsZero() bool {
	return s.Value.IsZero() == 1
}

func (s *ScalarBls12381Gt) IsOne() bool {
	return s.Value.IsOne() == 1
}

func (s *ScalarBls12381Gt) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarBls12381Gt) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12381Gt)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.Value = ss.Value
	return nil
}

func (s *ScalarBls12381Gt) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarBls12381Gt) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarBls12381Gt)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.Value = ss.Value
	return nil
}

func (s *ScalarBls12381Gt) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarBls12381Gt) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarBls12381Gt)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.Value = S.Value
	return nil
}

func (s *ScalarBls12381Gt) IsOdd() bool {
	data := s.Value.Bytes()
	return data[0]&1 == 1
}

func (s *ScalarBls12381Gt) IsEven() bool {
	data := s.Value.Bytes()
	return data[0]&1 == 0
}

func (s *ScalarBls12381Gt) New(input int) Scalar {
	var data [bls12381.GtFieldBytes]byte
	data[3] = byte(input >> 24 & 0xFF)
	data[2] = byte(input >> 16 & 0xFF)
	data[1] = byte(input >> 8 & 0xFF)
	data[0] = byte(input & 0xFF)

	value, isCanonical := new(bls12381.Gt).SetBytes(&data)
	if isCanonical != 1 {
		return nil
	}
	return &ScalarBls12381Gt{value}
}

func (s *ScalarBls12381Gt) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarBls12381Gt)
	if ok && s.Value.Equal(r.Value) == 1 {
		return 0
	} else {
		return -2
	}
}

func (s *ScalarBls12381Gt) Square() Scalar {
	return &ScalarBls12381Gt{
		new(bls12381.Gt).Square(s.Value),
	}
}

func (s *ScalarBls12381Gt) Double() Scalar {
	return &ScalarBls12381Gt{
		new(bls12381.Gt).Double(s.Value),
	}
}

func (s *ScalarBls12381Gt) Invert() (Scalar, error) {
	value, wasInverted := new(bls12381.Gt).Invert(s.Value)
	if wasInverted != 1 {
		return nil, fmt.Errorf("not invertible")
	}
	return &ScalarBls12381Gt{
		value,
	}, nil
}

func (s *ScalarBls12381Gt) Sqrt() (Scalar, error) {
	// Not implemented
	return nil, nil
}

func (s *ScalarBls12381Gt) Cube() Scalar {
	value := new(bls12381.Gt).Square(s.Value)
	value.Add(value, s.Value)
	return &ScalarBls12381Gt{
		value,
	}
}

func (s *ScalarBls12381Gt) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381Gt)
	if ok {
		return &ScalarBls12381Gt{
			new(bls12381.Gt).Add(s.Value, r.Value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381Gt) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381Gt)
	if ok {
		return &ScalarBls12381Gt{
			new(bls12381.Gt).Sub(s.Value, r.Value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381Gt) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381Gt)
	if ok {
		return &ScalarBls12381Gt{
			new(bls12381.Gt).Add(s.Value, r.Value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381Gt) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarBls12381Gt) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarBls12381Gt)
	if ok {
		return &ScalarBls12381Gt{
			new(bls12381.Gt).Sub(s.Value, r.Value),
		}
	} else {
		return nil
	}
}

func (s *ScalarBls12381Gt) Neg() Scalar {
	return &ScalarBls12381Gt{
		new(bls12381.Gt).Neg(s.Value),
	}
}

func (s *ScalarBls12381Gt) SetBigInt(v *big.Int) (Scalar, error) {
	var bytes [bls12381.GtFieldBytes]byte
	v.FillBytes(bytes[:])
	return s.SetBytes(bytes[:])
}

func (s *ScalarBls12381Gt) BigInt() *big.Int {
	bytes := s.Value.Bytes()
	return new(big.Int).SetBytes(bytes[:])
}

func (s *ScalarBls12381Gt) Point() Point {
	return &PointBls12381G1{Value: new(bls12381.G1).Identity()}
}

func (s *ScalarBls12381Gt) Bytes() []byte {
	bytes := s.Value.Bytes()
	return bytes[:]
}

func (s *ScalarBls12381Gt) SetBytes(bytes []byte) (Scalar, error) {
	var b [bls12381.GtFieldBytes]byte
	copy(b[:], bytes)
	ss, isCanonical := new(bls12381.Gt).SetBytes(&b)
	if isCanonical == 0 {
		return nil, fmt.Errorf("invalid bytes")
	}
	return &ScalarBls12381Gt{ss}, nil
}

func (s *ScalarBls12381Gt) SetBytesWide(bytes []byte) (Scalar, error) {
	l := len(bytes)
	if l != bls12381.GtFieldBytes*2 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	var b [bls12381.GtFieldBytes]byte
	copy(b[:], bytes[:bls12381.GtFieldBytes])

	value, isCanonical := new(bls12381.Gt).SetBytes(&b)
	if isCanonical == 0 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	copy(b[:], bytes[bls12381.GtFieldBytes:])
	value2, isCanonical := new(bls12381.Gt).SetBytes(&b)
	if isCanonical == 0 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	value.Add(value, value2)
	return &ScalarBls12381Gt{value}, nil
}

func (s *ScalarBls12381Gt) Clone() Scalar {
	return &ScalarBls12381Gt{
		Value: new(bls12381.Gt).Set(s.Value),
	}
}
