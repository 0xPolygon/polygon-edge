//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"crypto/elliptic"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/btcsuite/btcd/btcec"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	secp256k1 "github.com/coinbase/kryptology/pkg/core/curves/native/k256"
	"github.com/coinbase/kryptology/pkg/core/curves/native/k256/fp"
	"github.com/coinbase/kryptology/pkg/core/curves/native/k256/fq"
)

var oldK256Initonce sync.Once
var oldK256 Koblitz256

type Koblitz256 struct {
	*elliptic.CurveParams
}

func oldK256InitAll() {
	curve := btcec.S256()
	oldK256.CurveParams = new(elliptic.CurveParams)
	oldK256.P = curve.P
	oldK256.N = curve.N
	oldK256.Gx = curve.Gx
	oldK256.Gy = curve.Gy
	oldK256.B = curve.B
	oldK256.BitSize = curve.BitSize
	oldK256.Name = K256Name
}

func K256Curve() *Koblitz256 {
	oldK256Initonce.Do(oldK256InitAll)
	return &oldK256
}

func (curve *Koblitz256) Params() *elliptic.CurveParams {
	return curve.CurveParams
}

func (curve *Koblitz256) IsOnCurve(x, y *big.Int) bool {
	_, err := secp256k1.K256PointNew().SetBigInt(x, y)
	return err == nil
}

func (curve *Koblitz256) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	p1, err := secp256k1.K256PointNew().SetBigInt(x1, y1)
	if err != nil {
		return nil, nil
	}
	p2, err := secp256k1.K256PointNew().SetBigInt(x2, y2)
	if err != nil {
		return nil, nil
	}
	return p1.Add(p1, p2).BigInt()
}

func (curve *Koblitz256) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	p1, err := secp256k1.K256PointNew().SetBigInt(x1, y1)
	if err != nil {
		return nil, nil
	}
	return p1.Double(p1).BigInt()
}

func (curve *Koblitz256) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	p1, err := secp256k1.K256PointNew().SetBigInt(Bx, By)
	if err != nil {
		return nil, nil
	}
	var bytes [32]byte
	copy(bytes[:], internal.ReverseScalarBytes(k))
	s, err := fq.K256FqNew().SetBytes(&bytes)
	if err != nil {
		return nil, nil
	}
	return p1.Mul(p1, s).BigInt()
}

func (curve *Koblitz256) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	var bytes [32]byte
	copy(bytes[:], internal.ReverseScalarBytes(k))
	s, err := fq.K256FqNew().SetBytes(&bytes)
	if err != nil {
		return nil, nil
	}
	p1 := secp256k1.K256PointNew().Generator()
	return p1.Mul(p1, s).BigInt()
}

type ScalarK256 struct {
	value *native.Field
}

type PointK256 struct {
	value *native.EllipticPoint
}

func (s *ScalarK256) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarK256) Hash(bytes []byte) Scalar {
	dst := []byte("secp256k1_XMD:SHA-256_SSWU_RO_")
	xmd := native.ExpandMsgXmd(native.EllipticPointHasherSha256(), bytes, dst, 48)
	var t [64]byte
	copy(t[:48], internal.ReverseScalarBytes(xmd))

	return &ScalarK256{
		value: fq.K256FqNew().SetBytesWide(&t),
	}
}

func (s *ScalarK256) Zero() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().SetZero(),
	}
}

func (s *ScalarK256) One() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().SetOne(),
	}
}

func (s *ScalarK256) IsZero() bool {
	return s.value.IsZero() == 1
}

func (s *ScalarK256) IsOne() bool {
	return s.value.IsOne() == 1
}

func (s *ScalarK256) IsOdd() bool {
	return s.value.Bytes()[0]&1 == 1
}

func (s *ScalarK256) IsEven() bool {
	return s.value.Bytes()[0]&1 == 0
}

func (s *ScalarK256) New(value int) Scalar {
	t := fq.K256FqNew()
	v := big.NewInt(int64(value))
	if value < 0 {
		v.Mod(v, t.Params.BiModulus)
	}
	return &ScalarK256{
		value: t.SetBigInt(v),
	}
}

func (s *ScalarK256) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarK256)
	if ok {
		return s.value.Cmp(r.value)
	} else {
		return -2
	}
}

func (s *ScalarK256) Square() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().Square(s.value),
	}
}

func (s *ScalarK256) Double() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().Double(s.value),
	}
}

func (s *ScalarK256) Invert() (Scalar, error) {
	value, wasInverted := fq.K256FqNew().Invert(s.value)
	if !wasInverted {
		return nil, fmt.Errorf("inverse doesn't exist")
	}
	return &ScalarK256{
		value,
	}, nil
}

func (s *ScalarK256) Sqrt() (Scalar, error) {
	value, wasSquare := fq.K256FqNew().Sqrt(s.value)
	if !wasSquare {
		return nil, fmt.Errorf("not a square")
	}
	return &ScalarK256{
		value,
	}, nil
}

func (s *ScalarK256) Cube() Scalar {
	value := fq.K256FqNew().Mul(s.value, s.value)
	value.Mul(value, s.value)
	return &ScalarK256{
		value,
	}
}

func (s *ScalarK256) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarK256)
	if ok {
		return &ScalarK256{
			value: fq.K256FqNew().Add(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarK256) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarK256)
	if ok {
		return &ScalarK256{
			value: fq.K256FqNew().Sub(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarK256) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarK256)
	if ok {
		return &ScalarK256{
			value: fq.K256FqNew().Mul(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarK256) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarK256) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarK256)
	if ok {
		v, wasInverted := fq.K256FqNew().Invert(r.value)
		if !wasInverted {
			return nil
		}
		v.Mul(v, s.value)
		return &ScalarK256{value: v}
	} else {
		return nil
	}
}

func (s *ScalarK256) Neg() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().Neg(s.value),
	}
}

func (s *ScalarK256) SetBigInt(v *big.Int) (Scalar, error) {
	if v == nil {
		return nil, fmt.Errorf("'v' cannot be nil")
	}
	value := fq.K256FqNew().SetBigInt(v)
	return &ScalarK256{
		value,
	}, nil
}

func (s *ScalarK256) BigInt() *big.Int {
	return s.value.BigInt()
}

func (s *ScalarK256) Bytes() []byte {
	t := s.value.Bytes()
	return internal.ReverseScalarBytes(t[:])
}

func (s *ScalarK256) SetBytes(bytes []byte) (Scalar, error) {
	if len(bytes) != 32 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [32]byte
	copy(seq[:], internal.ReverseScalarBytes(bytes))
	value, err := fq.K256FqNew().SetBytes(&seq)
	if err != nil {
		return nil, err
	}
	return &ScalarK256{
		value,
	}, nil
}

func (s *ScalarK256) SetBytesWide(bytes []byte) (Scalar, error) {
	if len(bytes) != 64 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [64]byte
	copy(seq[:], bytes)
	return &ScalarK256{
		value: fq.K256FqNew().SetBytesWide(&seq),
	}, nil
}

func (s *ScalarK256) Point() Point {
	return new(PointK256).Identity()
}

func (s *ScalarK256) Clone() Scalar {
	return &ScalarK256{
		value: fq.K256FqNew().Set(s.value),
	}
}

func (s *ScalarK256) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarK256) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarK256)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarK256) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarK256) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarK256)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarK256) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarK256) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarK256)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

func (p *PointK256) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointK256) Hash(bytes []byte) Point {
	value, err := secp256k1.K256PointNew().Hash(bytes, native.EllipticPointHasherSha256())

	// TODO: change hash to return an error also
	if err != nil {
		return nil
	}

	return &PointK256{value}
}

func (p *PointK256) Identity() Point {
	return &PointK256{
		value: secp256k1.K256PointNew().Identity(),
	}
}

func (p *PointK256) Generator() Point {
	return &PointK256{
		value: secp256k1.K256PointNew().Generator(),
	}
}

func (p *PointK256) IsIdentity() bool {
	return p.value.IsIdentity()
}

func (p *PointK256) IsNegative() bool {
	return p.value.GetY().Value[0]&1 == 1
}

func (p *PointK256) IsOnCurve() bool {
	return p.value.IsOnCurve()
}

func (p *PointK256) Double() Point {
	value := secp256k1.K256PointNew().Double(p.value)
	return &PointK256{value}
}

func (p *PointK256) Scalar() Scalar {
	return new(ScalarK256).Zero()
}

func (p *PointK256) Neg() Point {
	value := secp256k1.K256PointNew().Neg(p.value)
	return &PointK256{value}
}

func (p *PointK256) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointK256)
	if ok {
		value := secp256k1.K256PointNew().Add(p.value, r.value)
		return &PointK256{value}
	} else {
		return nil
	}
}

func (p *PointK256) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointK256)
	if ok {
		value := secp256k1.K256PointNew().Sub(p.value, r.value)
		return &PointK256{value}
	} else {
		return nil
	}
}

func (p *PointK256) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarK256)
	if ok {
		value := secp256k1.K256PointNew().Mul(p.value, r.value)
		return &PointK256{value}
	} else {
		return nil
	}
}

func (p *PointK256) Equal(rhs Point) bool {
	r, ok := rhs.(*PointK256)
	if ok {
		return p.value.Equal(r.value) == 1
	} else {
		return false
	}
}

func (p *PointK256) Set(x, y *big.Int) (Point, error) {
	value, err := secp256k1.K256PointNew().SetBigInt(x, y)
	if err != nil {
		return nil, err
	}
	return &PointK256{value}, nil
}

func (p *PointK256) ToAffineCompressed() []byte {
	var x [33]byte
	x[0] = byte(2)

	t := secp256k1.K256PointNew().ToAffine(p.value)

	x[0] |= t.Y.Bytes()[0] & 1

	xBytes := t.X.Bytes()
	copy(x[1:], internal.ReverseScalarBytes(xBytes[:]))
	return x[:]
}

func (p *PointK256) ToAffineUncompressed() []byte {
	var out [65]byte
	out[0] = byte(4)
	t := secp256k1.K256PointNew().ToAffine(p.value)
	arr := t.X.Bytes()
	copy(out[1:33], internal.ReverseScalarBytes(arr[:]))
	arr = t.Y.Bytes()
	copy(out[33:], internal.ReverseScalarBytes(arr[:]))
	return out[:]
}

func (p *PointK256) FromAffineCompressed(bytes []byte) (Point, error) {
	var raw [native.FieldBytes]byte
	if len(bytes) != 33 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	sign := int(bytes[0])
	if sign != 2 && sign != 3 {
		return nil, fmt.Errorf("invalid sign byte")
	}
	sign &= 0x1

	copy(raw[:], internal.ReverseScalarBytes(bytes[1:]))
	x, err := fp.K256FpNew().SetBytes(&raw)
	if err != nil {
		return nil, err
	}

	value := secp256k1.K256PointNew().Identity()
	rhs := fp.K256FpNew()
	p.value.Arithmetic.RhsEq(rhs, x)
	// test that rhs is quadratic residue
	// if not, then this Point is at infinity
	y, wasQr := fp.K256FpNew().Sqrt(rhs)
	if wasQr {
		// fix the sign
		sigY := int(y.Bytes()[0] & 1)
		if sigY != sign {
			y.Neg(y)
		}
		value.X = x
		value.Y = y
		value.Z.SetOne()
	}
	return &PointK256{value}, nil
}

func (p *PointK256) FromAffineUncompressed(bytes []byte) (Point, error) {
	var arr [native.FieldBytes]byte
	if len(bytes) != 65 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	if bytes[0] != 4 {
		return nil, fmt.Errorf("invalid sign byte")
	}

	copy(arr[:], internal.ReverseScalarBytes(bytes[1:33]))
	x, err := fp.K256FpNew().SetBytes(&arr)
	if err != nil {
		return nil, err
	}
	copy(arr[:], internal.ReverseScalarBytes(bytes[33:]))
	y, err := fp.K256FpNew().SetBytes(&arr)
	if err != nil {
		return nil, err
	}
	value := secp256k1.K256PointNew()
	value.X = x
	value.Y = y
	value.Z.SetOne()
	return &PointK256{value}, nil
}

func (p *PointK256) CurveName() string {
	return p.value.Params.Name
}

func (p *PointK256) SumOfProducts(points []Point, scalars []Scalar) Point {
	nPoints := make([]*native.EllipticPoint, len(points))
	nScalars := make([]*native.Field, len(scalars))
	for i, pt := range points {
		ptv, ok := pt.(*PointK256)
		if !ok {
			return nil
		}
		nPoints[i] = ptv.value
	}
	for i, sc := range scalars {
		s, ok := sc.(*ScalarK256)
		if !ok {
			return nil
		}
		nScalars[i] = s.value
	}
	value := secp256k1.K256PointNew()
	_, err := value.SumOfProducts(nPoints, nScalars)
	if err != nil {
		return nil
	}
	return &PointK256{value}
}

func (p *PointK256) X() *native.Field {
	return p.value.GetX()
}

func (p *PointK256) Y() *native.Field {
	return p.value.GetY()
}

func (p *PointK256) Params() *elliptic.CurveParams {
	return K256Curve().Params()
}

func (p *PointK256) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointK256) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointK256)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointK256) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointK256) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointK256)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointK256) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointK256) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointK256)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}
