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

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	p256n "github.com/coinbase/kryptology/pkg/core/curves/native/p256"
	"github.com/coinbase/kryptology/pkg/core/curves/native/p256/fp"
	"github.com/coinbase/kryptology/pkg/core/curves/native/p256/fq"
)

var oldP256InitOnce sync.Once
var oldP256 NistP256

type NistP256 struct {
	*elliptic.CurveParams
}

func oldP256InitAll() {
	curve := elliptic.P256()
	oldP256.CurveParams = curve.Params()
	oldP256.P = curve.Params().P
	oldP256.N = curve.Params().N
	oldP256.Gx = curve.Params().Gx
	oldP256.Gy = curve.Params().Gy
	oldP256.B = curve.Params().B
	oldP256.BitSize = curve.Params().BitSize
	oldP256.Name = curve.Params().Name
}

func NistP256Curve() *NistP256 {
	oldP256InitOnce.Do(oldP256InitAll)
	return &oldP256
}

func (curve *NistP256) Params() *elliptic.CurveParams {
	return curve.CurveParams
}

func (curve *NistP256) IsOnCurve(x, y *big.Int) bool {
	_, err := p256n.P256PointNew().SetBigInt(x, y)
	return err == nil
}

func (curve *NistP256) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	p1, err := p256n.P256PointNew().SetBigInt(x1, y1)
	if err != nil {
		return nil, nil
	}
	p2, err := p256n.P256PointNew().SetBigInt(x2, y2)
	if err != nil {
		return nil, nil
	}
	return p1.Add(p1, p2).BigInt()
}

func (curve *NistP256) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	p1, err := p256n.P256PointNew().SetBigInt(x1, y1)
	if err != nil {
		return nil, nil
	}
	return p1.Double(p1).BigInt()
}

func (curve *NistP256) ScalarMul(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	p1, err := p256n.P256PointNew().SetBigInt(Bx, By)
	if err != nil {
		return nil, nil
	}
	var bytes [32]byte
	copy(bytes[:], internal.ReverseScalarBytes(k))
	s, err := fq.P256FqNew().SetBytes(&bytes)
	if err != nil {
		return nil, nil
	}
	return p1.Mul(p1, s).BigInt()
}

func (curve *NistP256) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	var bytes [32]byte
	copy(bytes[:], internal.ReverseScalarBytes(k))
	s, err := fq.P256FqNew().SetBytes(&bytes)
	if err != nil {
		return nil, nil
	}
	p1 := p256n.P256PointNew().Generator()
	return p1.Mul(p1, s).BigInt()
}

type ScalarP256 struct {
	value *native.Field
}

type PointP256 struct {
	value *native.EllipticPoint
}

func (s *ScalarP256) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarP256) Hash(bytes []byte) Scalar {
	dst := []byte("P256_XMD:SHA-256_SSWU_RO_")
	xmd := native.ExpandMsgXmd(native.EllipticPointHasherSha256(), bytes, dst, 48)
	var t [64]byte
	copy(t[:48], internal.ReverseScalarBytes(xmd))

	return &ScalarP256{
		value: fq.P256FqNew().SetBytesWide(&t),
	}
}

func (s *ScalarP256) Zero() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().SetZero(),
	}
}

func (s *ScalarP256) One() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().SetOne(),
	}
}

func (s *ScalarP256) IsZero() bool {
	return s.value.IsZero() == 1
}

func (s *ScalarP256) IsOne() bool {
	return s.value.IsOne() == 1
}

func (s *ScalarP256) IsOdd() bool {
	return s.value.Bytes()[0]&1 == 1
}

func (s *ScalarP256) IsEven() bool {
	return s.value.Bytes()[0]&1 == 0
}

func (s *ScalarP256) New(value int) Scalar {
	t := fq.P256FqNew()
	v := big.NewInt(int64(value))
	if value < 0 {
		v.Mod(v, t.Params.BiModulus)
	}
	return &ScalarP256{
		value: t.SetBigInt(v),
	}
}

func (s *ScalarP256) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarP256)
	if ok {
		return s.value.Cmp(r.value)
	} else {
		return -2
	}
}

func (s *ScalarP256) Square() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().Square(s.value),
	}
}

func (s *ScalarP256) Double() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().Double(s.value),
	}
}

func (s *ScalarP256) Invert() (Scalar, error) {
	value, wasInverted := fq.P256FqNew().Invert(s.value)
	if !wasInverted {
		return nil, fmt.Errorf("inverse doesn't exist")
	}
	return &ScalarP256{
		value,
	}, nil
}

func (s *ScalarP256) Sqrt() (Scalar, error) {
	value, wasSquare := fq.P256FqNew().Sqrt(s.value)
	if !wasSquare {
		return nil, fmt.Errorf("not a square")
	}
	return &ScalarP256{
		value,
	}, nil
}

func (s *ScalarP256) Cube() Scalar {
	value := fq.P256FqNew().Mul(s.value, s.value)
	value.Mul(value, s.value)
	return &ScalarP256{
		value,
	}
}

func (s *ScalarP256) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarP256)
	if ok {
		return &ScalarP256{
			value: fq.P256FqNew().Add(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarP256) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarP256)
	if ok {
		return &ScalarP256{
			value: fq.P256FqNew().Sub(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarP256) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarP256)
	if ok {
		return &ScalarP256{
			value: fq.P256FqNew().Mul(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarP256) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarP256) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarP256)
	if ok {
		v, wasInverted := fq.P256FqNew().Invert(r.value)
		if !wasInverted {
			return nil
		}
		v.Mul(v, s.value)
		return &ScalarP256{value: v}
	} else {
		return nil
	}
}

func (s *ScalarP256) Neg() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().Neg(s.value),
	}
}

func (s *ScalarP256) SetBigInt(v *big.Int) (Scalar, error) {
	if v == nil {
		return nil, fmt.Errorf("'v' cannot be nil")
	}
	value := fq.P256FqNew().SetBigInt(v)
	return &ScalarP256{
		value,
	}, nil
}

func (s *ScalarP256) BigInt() *big.Int {
	return s.value.BigInt()
}

func (s *ScalarP256) Bytes() []byte {
	t := s.value.Bytes()
	return internal.ReverseScalarBytes(t[:])
}

func (s *ScalarP256) SetBytes(bytes []byte) (Scalar, error) {
	if len(bytes) != 32 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [32]byte
	copy(seq[:], internal.ReverseScalarBytes(bytes))
	value, err := fq.P256FqNew().SetBytes(&seq)
	if err != nil {
		return nil, err
	}
	return &ScalarP256{
		value,
	}, nil
}

func (s *ScalarP256) SetBytesWide(bytes []byte) (Scalar, error) {
	if len(bytes) != 64 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [64]byte
	copy(seq[:], bytes)
	return &ScalarP256{
		value: fq.P256FqNew().SetBytesWide(&seq),
	}, nil
}

func (s *ScalarP256) Point() Point {
	return new(PointP256).Identity()
}

func (s *ScalarP256) Clone() Scalar {
	return &ScalarP256{
		value: fq.P256FqNew().Set(s.value),
	}
}

func (s *ScalarP256) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarP256) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarP256)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarP256) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarP256) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarP256)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarP256) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarP256) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarP256)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

func (p *PointP256) Random(reader io.Reader) Point {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *PointP256) Hash(bytes []byte) Point {
	value, err := p256n.P256PointNew().Hash(bytes, native.EllipticPointHasherSha256())

	// TODO: change hash to return an error also
	if err != nil {
		return nil
	}

	return &PointP256{value}
}

func (p *PointP256) Identity() Point {
	return &PointP256{
		value: p256n.P256PointNew().Identity(),
	}
}

func (p *PointP256) Generator() Point {
	return &PointP256{
		value: p256n.P256PointNew().Generator(),
	}
}

func (p *PointP256) IsIdentity() bool {
	return p.value.IsIdentity()
}

func (p *PointP256) IsNegative() bool {
	return p.value.GetY().Value[0]&1 == 1
}

func (p *PointP256) IsOnCurve() bool {
	return p.value.IsOnCurve()
}

func (p *PointP256) Double() Point {
	value := p256n.P256PointNew().Double(p.value)
	return &PointP256{value}
}

func (p *PointP256) Scalar() Scalar {
	return new(ScalarP256).Zero()
}

func (p *PointP256) Neg() Point {
	value := p256n.P256PointNew().Neg(p.value)
	return &PointP256{value}
}

func (p *PointP256) Add(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointP256)
	if ok {
		value := p256n.P256PointNew().Add(p.value, r.value)
		return &PointP256{value}
	} else {
		return nil
	}
}

func (p *PointP256) Sub(rhs Point) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*PointP256)
	if ok {
		value := p256n.P256PointNew().Sub(p.value, r.value)
		return &PointP256{value}
	} else {
		return nil
	}
}

func (p *PointP256) Mul(rhs Scalar) Point {
	if rhs == nil {
		return nil
	}
	r, ok := rhs.(*ScalarP256)
	if ok {
		value := p256n.P256PointNew().Mul(p.value, r.value)
		return &PointP256{value}
	} else {
		return nil
	}
}

func (p *PointP256) Equal(rhs Point) bool {
	r, ok := rhs.(*PointP256)
	if ok {
		return p.value.Equal(r.value) == 1
	} else {
		return false
	}
}

func (p *PointP256) Set(x, y *big.Int) (Point, error) {
	value, err := p256n.P256PointNew().SetBigInt(x, y)
	if err != nil {
		return nil, err
	}
	return &PointP256{value}, nil
}

func (p *PointP256) ToAffineCompressed() []byte {
	var x [33]byte
	x[0] = byte(2)

	t := p256n.P256PointNew().ToAffine(p.value)

	x[0] |= t.Y.Bytes()[0] & 1

	xBytes := t.X.Bytes()
	copy(x[1:], internal.ReverseScalarBytes(xBytes[:]))
	return x[:]
}

func (p *PointP256) ToAffineUncompressed() []byte {
	var out [65]byte
	out[0] = byte(4)
	t := p256n.P256PointNew().ToAffine(p.value)
	arr := t.X.Bytes()
	copy(out[1:33], internal.ReverseScalarBytes(arr[:]))
	arr = t.Y.Bytes()
	copy(out[33:], internal.ReverseScalarBytes(arr[:]))
	return out[:]
}

func (p *PointP256) FromAffineCompressed(bytes []byte) (Point, error) {
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
	x, err := fp.P256FpNew().SetBytes(&raw)
	if err != nil {
		return nil, err
	}

	value := p256n.P256PointNew().Identity()
	rhs := fp.P256FpNew()
	p.value.Arithmetic.RhsEq(rhs, x)
	// test that rhs is quadratic residue
	// if not, then this Point is at infinity
	y, wasQr := fp.P256FpNew().Sqrt(rhs)
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
	return &PointP256{value}, nil
}

func (p *PointP256) FromAffineUncompressed(bytes []byte) (Point, error) {
	var arr [native.FieldBytes]byte
	if len(bytes) != 65 {
		return nil, fmt.Errorf("invalid byte sequence")
	}
	if bytes[0] != 4 {
		return nil, fmt.Errorf("invalid sign byte")
	}

	copy(arr[:], internal.ReverseScalarBytes(bytes[1:33]))
	x, err := fp.P256FpNew().SetBytes(&arr)
	if err != nil {
		return nil, err
	}
	copy(arr[:], internal.ReverseScalarBytes(bytes[33:]))
	y, err := fp.P256FpNew().SetBytes(&arr)
	if err != nil {
		return nil, err
	}
	value := p256n.P256PointNew()
	value.X = x
	value.Y = y
	value.Z.SetOne()
	return &PointP256{value}, nil
}

func (p *PointP256) CurveName() string {
	return elliptic.P256().Params().Name
}

func (p *PointP256) SumOfProducts(points []Point, scalars []Scalar) Point {
	nPoints := make([]*native.EllipticPoint, len(points))
	nScalars := make([]*native.Field, len(scalars))
	for i, pt := range points {
		ptv, ok := pt.(*PointP256)
		if !ok {
			return nil
		}
		nPoints[i] = ptv.value
	}
	for i, sc := range scalars {
		s, ok := sc.(*ScalarP256)
		if !ok {
			return nil
		}
		nScalars[i] = s.value
	}
	value := p256n.P256PointNew()
	_, err := value.SumOfProducts(nPoints, nScalars)
	if err != nil {
		return nil
	}
	return &PointP256{value}
}

func (p *PointP256) X() *native.Field {
	return p.value.GetX()
}

func (p *PointP256) Y() *native.Field {
	return p.value.GetY()
}

func (p *PointP256) Params() *elliptic.CurveParams {
	return elliptic.P256().Params()
}

func (p *PointP256) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointP256) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointP256)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointP256) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointP256) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointP256)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointP256) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointP256) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointP256)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}
