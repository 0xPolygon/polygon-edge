//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"bytes"
	"crypto/sha512"
	"crypto/subtle"
	"fmt"
	"io"
	"math/big"

	"filippo.io/edwards25519"
	"filippo.io/edwards25519/field"
	"github.com/bwesterb/go-ristretto"
	ed "github.com/bwesterb/go-ristretto/edwards25519"

	"github.com/coinbase/kryptology/internal"
)

type ScalarEd25519 struct {
	value *edwards25519.Scalar
}

type PointEd25519 struct {
	value *edwards25519.Point
}

var scOne, _ = edwards25519.NewScalar().SetCanonicalBytes([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})

func (s *ScalarEd25519) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarEd25519) Hash(bytes []byte) Scalar {
	v := new(ristretto.Scalar).Derive(bytes)
	var data [32]byte
	v.BytesInto(&data)
	value, err := edwards25519.NewScalar().SetCanonicalBytes(data[:])
	if err != nil {
		return nil
	}
	return &ScalarEd25519{value}
}

func (s *ScalarEd25519) Zero() Scalar {
	return &ScalarEd25519{
		value: edwards25519.NewScalar(),
	}
}

func (s *ScalarEd25519) One() Scalar {
	return &ScalarEd25519{
		value: edwards25519.NewScalar().Set(scOne),
	}
}

func (s *ScalarEd25519) IsZero() bool {
	i := byte(0)
	for _, b := range s.value.Bytes() {
		i |= b
	}
	return i == 0
}

func (s *ScalarEd25519) IsOne() bool {
	data := s.value.Bytes()
	i := byte(0)
	for j := 1; j < len(data); j++ {
		i |= data[j]
	}
	return i == 0 && data[0] == 1
}

func (s *ScalarEd25519) IsOdd() bool {
	return s.value.Bytes()[0]&1 == 1
}

func (s *ScalarEd25519) IsEven() bool {
	return s.value.Bytes()[0]&1 == 0
}

func (s *ScalarEd25519) New(input int) Scalar {
	var data [64]byte
	i := input
	if input < 0 {
		i = -input
	}
	data[0] = byte(i)
	data[1] = byte(i >> 8)
	data[2] = byte(i >> 16)
	data[3] = byte(i >> 24)
	value, err := edwards25519.NewScalar().SetUniformBytes(data[:])
	if err != nil {
		return nil
	}
	if input < 0 {
		value.Negate(value)
	}

	return &ScalarEd25519{
		value,
	}
}

func (s *ScalarEd25519) Cmp(rhs Scalar) int {
	r := s.Sub(rhs)
	if r != nil && r.IsZero() {
		return 0
	} else {
		return -2
	}
}

func (s *ScalarEd25519) Square() Scalar {
	value := edwards25519.NewScalar().Multiply(s.value, s.value)
	return &ScalarEd25519{value}
}

func (s *ScalarEd25519) Double() Scalar {
	return &ScalarEd25519{
		value: edwards25519.NewScalar().Add(s.value, s.value),
	}
}

func (s *ScalarEd25519) Invert() (Scalar, error) {
	return &ScalarEd25519{
		value: edwards25519.NewScalar().Invert(s.value),
	}, nil
}

func (s *ScalarEd25519) Sqrt() (Scalar, error) {
	bi25519, _ := new(big.Int).SetString("1000000000000000000000000000000014DEF9DEA2F79CD65812631A5CF5D3ED", 16)
	x := s.BigInt()
	x.ModSqrt(x, bi25519)
	return s.SetBigInt(x)
}

func (s *ScalarEd25519) Cube() Scalar {
	value := edwards25519.NewScalar().Multiply(s.value, s.value)
	value.Multiply(value, s.value)
	return &ScalarEd25519{value}
}

func (s *ScalarEd25519) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarEd25519)
	if ok {
		return &ScalarEd25519{
			value: edwards25519.NewScalar().Add(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarEd25519) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarEd25519)
	if ok {
		return &ScalarEd25519{
			value: edwards25519.NewScalar().Subtract(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarEd25519) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarEd25519)
	if ok {
		return &ScalarEd25519{
			value: edwards25519.NewScalar().Multiply(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarEd25519) MulAdd(y, z Scalar) Scalar {
	yy, ok := y.(*ScalarEd25519)
	if !ok {
		return nil
	}
	zz, ok := z.(*ScalarEd25519)
	if !ok {
		return nil
	}
	return &ScalarEd25519{value: edwards25519.NewScalar().MultiplyAdd(s.value, yy.value, zz.value)}
}

func (s *ScalarEd25519) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarEd25519)
	if ok {
		value := edwards25519.NewScalar().Invert(r.value)
		value.Multiply(value, s.value)
		return &ScalarEd25519{value}
	} else {
		return nil
	}
}

func (s *ScalarEd25519) Neg() Scalar {
	return &ScalarEd25519{
		value: edwards25519.NewScalar().Negate(s.value),
	}
}

func (s *ScalarEd25519) SetBigInt(x *big.Int) (Scalar, error) {
	if x == nil {
		return nil, fmt.Errorf("invalid value")
	}

	bi25519, _ := new(big.Int).SetString("1000000000000000000000000000000014DEF9DEA2F79CD65812631A5CF5D3ED", 16)
	var v big.Int
	buf := v.Mod(x, bi25519).Bytes()
	var rBuf [32]byte
	for i := 0; i < len(buf) && i < 32; i++ {
		rBuf[i] = buf[len(buf)-i-1]
	}
	value, err := edwards25519.NewScalar().SetCanonicalBytes(rBuf[:])
	if err != nil {
		return nil, err
	}
	return &ScalarEd25519{value}, nil
}

func (s *ScalarEd25519) BigInt() *big.Int {
	var ret big.Int
	buf := internal.ReverseScalarBytes(s.value.Bytes())
	return ret.SetBytes(buf)
}

func (s *ScalarEd25519) Bytes() []byte {
	return s.value.Bytes()
}

// SetBytes takes input a 32-byte long array and returns a ed25519 scalar.
// The input must be 32-byte long and must be a reduced bytes.
func (s *ScalarEd25519) SetBytes(input []byte) (Scalar, error) {
	if len(input) != 32 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	value, err := edwards25519.NewScalar().SetCanonicalBytes(input)
	if err != nil {
		return nil, err
	}
	return &ScalarEd25519{value}, nil
}

// SetBytesWide takes input a 64-byte long byte array, reduce it and return an ed25519 scalar.
// It uses SetUniformBytes of fillipo.io/edwards25519 - https://github.com/FiloSottile/edwards25519/blob/v1.0.0-rc.1/scalar.go#L85
// If bytes is not of the right length, it returns nil and an error
func (s *ScalarEd25519) SetBytesWide(bytes []byte) (Scalar, error) {
	value, err := edwards25519.NewScalar().SetUniformBytes(bytes)
	if err != nil {
		return nil, err
	}
	return &ScalarEd25519{value}, nil
}

// SetBytesClamping uses SetBytesWithClamping of fillipo.io/edwards25519- https://github.com/FiloSottile/edwards25519/blob/v1.0.0-rc.1/scalar.go#L135
// which applies the buffer pruning described in RFC 8032, Section 5.1.5 (also known as clamping)
// and sets bytes to the result. The input must be 32-byte long, and it is not modified.
// If bytes is not of the right length, SetBytesWithClamping returns nil and an error, and the receiver is unchanged.
func (s *ScalarEd25519) SetBytesClamping(bytes []byte) (Scalar, error) {
	value, err := edwards25519.NewScalar().SetBytesWithClamping(bytes)
	if err != nil {
		return nil, err
	}
	return &ScalarEd25519{value}, nil
}

// SetBytesCanonical uses SetCanonicalBytes of fillipo.io/edwards25519.
// https://github.com/FiloSottile/edwards25519/blob/v1.0.0-rc.1/scalar.go#L98
// This function takes an input x and sets s = x, where x is a 32-byte little-endian
// encoding of s, then it returns the corresponding ed25519 scalar. If the input is
// not a canonical encoding of s, it returns nil and an error.
func (s *ScalarEd25519) SetBytesCanonical(bytes []byte) (Scalar, error) {
	return s.SetBytes(bytes)
}

func (s *ScalarEd25519) Point() Point {
	return new(PointEd25519).Identity()
}

func (s *ScalarEd25519) Clone() Scalar {
	return &ScalarEd25519{
		value: edwards25519.NewScalar().Set(s.value),
	}
}

func (s *ScalarEd25519) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarEd25519) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarEd25519)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarEd25519) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarEd25519) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarEd25519)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarEd25519) GetEdwardsScalar() *edwards25519.Scalar {
	return edwards25519.NewScalar().Set(s.value)
}

func (s *ScalarEd25519) SetEdwardsScalar(sc *edwards25519.Scalar) *ScalarEd25519 {
	return &ScalarEd25519{value: edwards25519.NewScalar().Set(sc)}
}

func (s *ScalarEd25519) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarEd25519) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarEd25519)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

func (p *PointEd25519) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointEd25519) Hash(bytes []byte) Point {
	/// Perform hashing to the group using the Elligator2 map
	///
	/// See https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-11#section-6.7.1
	h := sha512.Sum512(bytes)
	var res [32]byte
	copy(res[:], h[:32])
	signBit := (res[31] & 0x80) >> 7

	fe := new(ed.FieldElement).SetBytes(&res).BytesInto(&res)
	m1 := elligatorEncode(fe)

	return toEdwards(m1, signBit)
}

func (p *PointEd25519) Identity() Point {
	return &PointEd25519{
		value: edwards25519.NewIdentityPoint(),
	}
}

func (p *PointEd25519) Generator() Point {
	return &PointEd25519{
		value: edwards25519.NewGeneratorPoint(),
	}
}

func (p *PointEd25519) IsIdentity() bool {
	return p.Equal(p.Identity())
}

func (p *PointEd25519) IsNegative() bool {
	// Negative points don't really exist in ed25519
	return false
}

func (p *PointEd25519) IsOnCurve() bool {
	_, err := edwards25519.NewIdentityPoint().SetBytes(p.ToAffineCompressed())
	return err == nil
}

func (p *PointEd25519) Double() Point {
	return &PointEd25519{value: edwards25519.NewIdentityPoint().Add(p.value, p.value)}
}

func (p *PointEd25519) Scalar() Scalar {
	return new(ScalarEd25519).Zero()
}

func (p *PointEd25519) Neg() Point {
	return &PointEd25519{value: edwards25519.NewIdentityPoint().Negate(p.value)}
}

func (p *PointEd25519) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointEd25519)
	if ok {
		return &PointEd25519{value: edwards25519.NewIdentityPoint().Add(p.value, r.value)}
	} else {
		return nil
	}
}

func (p *PointEd25519) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointEd25519)
	if ok {
		rTmp := edwards25519.NewIdentityPoint().Negate(r.value)
		return &PointEd25519{value: edwards25519.NewIdentityPoint().Add(p.value, rTmp)}
	} else {
		return nil
	}
}

func (p *PointEd25519) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarEd25519)
	if ok {
		value := edwards25519.NewIdentityPoint().ScalarMult(r.value, p.value)
		return &PointEd25519{value}
	} else {
		return nil
	}
}

// MangleScalarBitsAndMulByBasepointToProducePublicKey
// is a function for mangling the bits of a (formerly
// mathematically well-defined) "scalar" and multiplying it to produce a
// public key.
func (p *PointEd25519) MangleScalarBitsAndMulByBasepointToProducePublicKey(rhs *ScalarEd25519) *PointEd25519 {
	data := rhs.value.Bytes()
	s, err := edwards25519.NewScalar().SetBytesWithClamping(data[:])
	if err != nil {
		return nil
	}
	value := edwards25519.NewIdentityPoint().ScalarBaseMult(s)
	return &PointEd25519{value}
}

func (p *PointEd25519) Equal(rhs Point) bool {
	r, ok := rhs.(*PointEd25519)
	if ok {
		// We would like to check that the point (X/Z, Y/Z) is equal to
		// the point (X'/Z', Y'/Z') without converting into affine
		// coordinates (x, y) and (x', y'), which requires two inversions.
		// We have that X = xZ and X' = x'Z'. Thus, x = x' is equivalent to
		// (xZ)Z' = (x'Z')Z, and similarly for the y-coordinate.
		return p.value.Equal(r.value) == 1
		//lhs1 := new(ed.FieldElement).Mul(&p.value.X, &r.value.Z)
		//rhs1 := new(ed.FieldElement).Mul(&r.value.X, &p.value.Z)
		//lhs2 := new(ed.FieldElement).Mul(&p.value.Y, &r.value.Z)
		//rhs2 := new(ed.FieldElement).Mul(&r.value.Y, &p.value.Z)
		//
		//return lhs1.Equals(rhs1) && lhs2.Equals(rhs2)
	} else {
		return false
	}
}

func (p *PointEd25519) Set(x, y *big.Int) (Point, error) {
	// check is identity
	xx := subtle.ConstantTimeCompare(x.Bytes(), []byte{})
	yy := subtle.ConstantTimeCompare(y.Bytes(), []byte{})
	if (xx | yy) == 1 {
		return p.Identity(), nil
	}
	xElem := new(ed.FieldElement).SetBigInt(x)
	yElem := new(ed.FieldElement).SetBigInt(y)

	var data [32]byte
	var affine [64]byte
	xElem.BytesInto(&data)
	copy(affine[:32], data[:])
	yElem.BytesInto(&data)
	copy(affine[32:], data[:])
	return p.FromAffineUncompressed(affine[:])
}

// sqrtRatio sets r to the non-negative square root of the ratio of u and v.
//
// If u/v is square, sqrtRatio returns r and 1. If u/v is not square, SqrtRatio
// sets r according to Section 4.3 of draft-irtf-cfrg-ristretto255-decaf448-00,
// and returns r and 0.
func sqrtRatio(u, v *ed.FieldElement) (r *ed.FieldElement, wasSquare bool) {
	var sqrtM1 = ed.FieldElement{
		533094393274173, 2016890930128738, 18285341111199,
		134597186663265, 1486323764102114,
	}
	a := new(ed.FieldElement)
	b := new(ed.FieldElement)
	r = new(ed.FieldElement)

	// r = (u * v3) * (u * v7)^((p-5)/8)
	v2 := a.Square(v)
	uv3 := b.Mul(u, b.Mul(v2, v))
	uv7 := a.Mul(uv3, a.Square(v2))
	r.Mul(uv3, r.Exp22523(uv7))

	check := a.Mul(v, a.Square(r)) // check = v * r^2

	uNeg := b.Neg(u)
	correctSignSqrt := check.Equals(u)
	flippedSignSqrt := check.Equals(uNeg)
	flippedSignSqrtI := check.Equals(uNeg.Mul(uNeg, &sqrtM1))

	rPrime := b.Mul(r, &sqrtM1) // r_prime = SQRT_M1 * r
	// r = CT_SELECT(r_prime IF flipped_sign_sqrt | flipped_sign_sqrt_i ELSE r)
	cselect(r, rPrime, r, flippedSignSqrt || flippedSignSqrtI)

	r.Abs(r) // Choose the nonnegative square root.
	return r, correctSignSqrt || flippedSignSqrt
}

// cselect sets v to a if cond == 1, and to b if cond == 0.
func cselect(v, a, b *ed.FieldElement, cond bool) *ed.FieldElement {
	const mask64Bits uint64 = (1 << 64) - 1

	m := uint64(0)
	if cond {
		m = mask64Bits
	}

	v[0] = (m & a[0]) | (^m & b[0])
	v[1] = (m & a[1]) | (^m & b[1])
	v[2] = (m & a[2]) | (^m & b[2])
	v[3] = (m & a[3]) | (^m & b[3])
	v[4] = (m & a[4]) | (^m & b[4])
	return v
}

func (p *PointEd25519) ToAffineCompressed() []byte {
	return p.value.Bytes()
}

func (p *PointEd25519) ToAffineUncompressed() []byte {
	x, y, z, _ := p.value.ExtendedCoordinates()
	recip := new(field.Element).Invert(z)
	x.Multiply(x, recip)
	y.Multiply(y, recip)
	var out [64]byte
	copy(out[:32], x.Bytes())
	copy(out[32:], y.Bytes())
	return out[:]
}

func (p *PointEd25519) FromAffineCompressed(inBytes []byte) (Point, error) {
	pt, err := edwards25519.NewIdentityPoint().SetBytes(inBytes)
	if err != nil {
		return nil, err
	}
	return &PointEd25519{value: pt}, nil
}

func (p *PointEd25519) FromAffineUncompressed(inBytes []byte) (Point, error) {
	if len(inBytes) != 64 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	if bytes.Equal(inBytes, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		return &PointEd25519{value: edwards25519.NewIdentityPoint()}, nil
	}
	x, err := new(field.Element).SetBytes(inBytes[:32])
	if err != nil {
		return nil, err
	}
	y, err := new(field.Element).SetBytes(inBytes[32:])
	if err != nil {
		return nil, err
	}
	z := new(field.Element).One()
	t := new(field.Element).Multiply(x, y)
	value, err := edwards25519.NewIdentityPoint().SetExtendedCoordinates(x, y, z, t)
	if err != nil {
		return nil, err
	}
	return &PointEd25519{value}, nil
}

func (p *PointEd25519) CurveName() string {
	return ED25519Name
}

func (p *PointEd25519) SumOfProducts(points []Point, scalars []Scalar) Point {
	nScalars := make([]*edwards25519.Scalar, len(scalars))
	nPoints := make([]*edwards25519.Point, len(points))
	for i, sc := range scalars {
		s, err := edwards25519.NewScalar().SetCanonicalBytes(sc.Bytes())
		if err != nil {
			return nil
		}
		nScalars[i] = s
	}
	for i, pt := range points {
		pp, ok := pt.(*PointEd25519)
		if !ok {
			return nil
		}
		nPoints[i] = pp.value
	}
	pt := edwards25519.NewIdentityPoint().MultiScalarMult(nScalars, nPoints)
	return &PointEd25519{value: pt}
}

func (p *PointEd25519) VarTimeDoubleScalarBaseMult(a Scalar, A Point, b Scalar) Point {
	AA, ok := A.(*PointEd25519)
	if !ok {
		return nil
	}
	aa, ok := a.(*ScalarEd25519)
	if !ok {
		return nil
	}
	bb, ok := b.(*ScalarEd25519)
	if !ok {
		return nil
	}
	value := edwards25519.NewIdentityPoint().VarTimeDoubleScalarBaseMult(aa.value, AA.value, bb.value)
	return &PointEd25519{value}
}

func (p *PointEd25519) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointEd25519) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointEd25519)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointEd25519) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointEd25519) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointEd25519)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointEd25519) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointEd25519) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointEd25519)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}

func (p *PointEd25519) GetEdwardsPoint() *edwards25519.Point {
	return edwards25519.NewIdentityPoint().Set(p.value)
}

func (p *PointEd25519) SetEdwardsPoint(pt *edwards25519.Point) *PointEd25519 {
	return &PointEd25519{value: edwards25519.NewIdentityPoint().Set(pt)}
}

// Attempt to convert to an `EdwardsPoint`, using the supplied
// choice of sign for the `EdwardsPoint`.
// * `sign`: a `u8` donating the desired sign of the resulting
//   `EdwardsPoint`.  `0` denotes positive and `1` negative.
func toEdwards(u *ed.FieldElement, sign byte) *PointEd25519 {
	one := new(ed.FieldElement).SetOne()
	// To decompress the Montgomery u coordinate to an
	// `EdwardsPoint`, we apply the birational map to obtain the
	// Edwards y coordinate, then do Edwards decompression.
	//
	// The birational map is y = (u-1)/(u+1).
	//
	// The exceptional points are the zeros of the denominator,
	// i.e., u = -1.
	//
	// But when u = -1, v^2 = u*(u^2+486662*u+1) = 486660.
	//
	// Since this is nonsquare mod p, u = -1 corresponds to a point
	// on the twist, not the curve, so we can reject it early.
	if u.Equals(new(ed.FieldElement).Neg(one)) {
		return nil
	}

	// y = (u-1)/(u+1)
	yLhs := new(ed.FieldElement).Sub(u, one)
	yRhs := new(ed.FieldElement).Add(u, one)
	yInv := new(ed.FieldElement).Inverse(yRhs)
	y := new(ed.FieldElement).Mul(yLhs, yInv)
	yBytes := y.Bytes()
	yBytes[31] ^= sign << 7

	pt, err := edwards25519.NewIdentityPoint().SetBytes(yBytes[:])
	if err != nil {
		return nil
	}
	pt.MultByCofactor(pt)
	return &PointEd25519{value: pt}
}

// Perform the Elligator2 mapping to a Montgomery point encoded as a 32 byte value
//
// See <https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-11#section-6.7.1>
func elligatorEncode(r0 *ed.FieldElement) *ed.FieldElement {
	montgomeryA := &ed.FieldElement{
		486662, 0, 0, 0, 0,
	}
	// montgomeryANeg is equal to -486662.
	montgomeryANeg := &ed.FieldElement{2251799813198567,
		2251799813685247,
		2251799813685247,
		2251799813685247,
		2251799813685247}
	t := new(ed.FieldElement)
	one := new(ed.FieldElement).SetOne()
	// 2r^2
	d1 := new(ed.FieldElement).Add(one, t.DoubledSquare(r0))
	// A/(1+2r^2)
	d := new(ed.FieldElement).Mul(montgomeryANeg, t.Inverse(d1))
	dsq := new(ed.FieldElement).Square(d)
	au := new(ed.FieldElement).Mul(montgomeryA, d)

	inner := new(ed.FieldElement).Add(dsq, au)
	inner.Add(inner, one)

	// d^3 + Ad^2 + d
	eps := new(ed.FieldElement).Mul(d, inner)
	_, wasSquare := sqrtRatio(eps, one)

	zero := new(ed.FieldElement).SetZero()
	aTemp := new(ed.FieldElement).SetZero()
	// 0 or A if non-square
	cselect(aTemp, zero, montgomeryA, wasSquare)
	// d, or d+A if non-square
	u := new(ed.FieldElement).Add(d, aTemp)
	// d or -d-A if non-square
	cselect(u, u, new(ed.FieldElement).Neg(u), wasSquare)
	return u
}
