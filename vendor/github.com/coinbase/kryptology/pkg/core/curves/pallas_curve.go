//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package curves

import (
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"golang.org/x/crypto/blake2b"

	"github.com/coinbase/kryptology/pkg/core/curves/native/pasta/fp"
	"github.com/coinbase/kryptology/pkg/core/curves/native/pasta/fq"
)

var b = new(fp.Fp).SetUint64(5)
var three = &fp.Fp{0x6b0ee5d0fffffff5, 0x86f76d2b99b14bd0, 0xfffffffffffffffe, 0x3fffffffffffffff}
var eight = &fp.Fp{0x7387134cffffffe1, 0xd973797adfadd5a8, 0xfffffffffffffffb, 0x3fffffffffffffff}
var bool2int = map[bool]int{
	true:  1,
	false: 0,
}

var isomapper = [13]*fp.Fp{
	new(fp.Fp).SetRaw(&[4]uint64{0x775f6034aaaaaaab, 0x4081775473d8375b, 0xe38e38e38e38e38e, 0x0e38e38e38e38e38}),
	new(fp.Fp).SetRaw(&[4]uint64{0x8cf863b02814fb76, 0x0f93b82ee4b99495, 0x267c7ffa51cf412a, 0x3509afd51872d88e}),
	new(fp.Fp).SetRaw(&[4]uint64{0x0eb64faef37ea4f7, 0x380af066cfeb6d69, 0x98c7d7ac3d98fd13, 0x17329b9ec5253753}),
	new(fp.Fp).SetRaw(&[4]uint64{0xeebec06955555580, 0x8102eea8e7b06eb6, 0xc71c71c71c71c71c, 0x1c71c71c71c71c71}),
	new(fp.Fp).SetRaw(&[4]uint64{0xc47f2ab668bcd71f, 0x9c434ac1c96b6980, 0x5a607fcce0494a79, 0x1d572e7ddc099cff}),
	new(fp.Fp).SetRaw(&[4]uint64{0x2aa3af1eae5b6604, 0xb4abf9fb9a1fc81c, 0x1d13bf2a7f22b105, 0x325669becaecd5d1}),
	new(fp.Fp).SetRaw(&[4]uint64{0x5ad985b5e38e38e4, 0x7642b01ad461bad2, 0x4bda12f684bda12f, 0x1a12f684bda12f68}),
	new(fp.Fp).SetRaw(&[4]uint64{0xc67c31d8140a7dbb, 0x07c9dc17725cca4a, 0x133e3ffd28e7a095, 0x1a84d7ea8c396c47}),
	new(fp.Fp).SetRaw(&[4]uint64{0x02e2be87d225b234, 0x1765e924f7459378, 0x303216cce1db9ff1, 0x3fb98ff0d2ddcadd}),
	new(fp.Fp).SetRaw(&[4]uint64{0x93e53ab371c71c4f, 0x0ac03e8e134eb3e4, 0x7b425ed097b425ed, 0x025ed097b425ed09}),
	new(fp.Fp).SetRaw(&[4]uint64{0x5a28279b1d1b42ae, 0x5941a3a4a97aa1b3, 0x0790bfb3506defb6, 0x0c02c5bcca0e6b7f}),
	new(fp.Fp).SetRaw(&[4]uint64{0x4d90ab820b12320a, 0xd976bbfabbc5661d, 0x573b3d7f7d681310, 0x17033d3c60c68173}),
	new(fp.Fp).SetRaw(&[4]uint64{0x992d30ecfffffde5, 0x224698fc094cf91b, 0x0000000000000000, 0x4000000000000000}),
}
var isoa = new(fp.Fp).SetRaw(&[4]uint64{0x92bb4b0b657a014b, 0xb74134581a27a59f, 0x49be2d7258370742, 0x18354a2eb0ea8c9c})
var isob = new(fp.Fp).SetRaw(&[4]uint64{1265, 0, 0, 0})
var z = new(fp.Fp).SetRaw(&[4]uint64{0x992d30ecfffffff4, 0x224698fc094cf91b, 0x0000000000000000, 0x4000000000000000})

var oldPallasInitonce sync.Once
var oldPallas PallasCurve

type PallasCurve struct {
	*elliptic.CurveParams
}

func oldPallasInitAll() {
	oldPallas.CurveParams = new(elliptic.CurveParams)
	oldPallas.P = new(big.Int).SetBytes([]byte{
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x22, 0x46, 0x98, 0xfc, 0x09, 0x4c, 0xf9, 0x1b,
		0x99, 0x2d, 0x30, 0xed, 0x00, 0x00, 0x00, 0x01,
	})
	oldPallas.N = new(big.Int).SetBytes([]byte{
		0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x22, 0x46, 0x98, 0xfc, 0x09, 0x94, 0xa8, 0xdd,
		0x8c, 0x46, 0xeb, 0x21, 0x00, 0x00, 0x00, 0x01,
	})
	g := new(Ep).Generator()
	oldPallas.Gx = g.x.BigInt()
	oldPallas.Gy = g.y.BigInt()
	oldPallas.B = big.NewInt(5)
	oldPallas.BitSize = 255
	pallas.Name = PallasName
}

func Pallas() *PallasCurve {
	oldPallasInitonce.Do(oldPallasInitAll)
	return &oldPallas
}

func (curve *PallasCurve) Params() *elliptic.CurveParams {
	return curve.CurveParams
}

func (curve *PallasCurve) IsOnCurve(x, y *big.Int) bool {
	p := new(Ep)
	p.x = new(fp.Fp).SetBigInt(x)
	p.y = new(fp.Fp).SetBigInt(y)
	p.z = new(fp.Fp).SetOne()

	return p.IsOnCurve()
}

func (curve *PallasCurve) Add(x1, y1, x2, y2 *big.Int) (*big.Int, *big.Int) {
	p := new(Ep)
	p.x = new(fp.Fp).SetBigInt(x1)
	p.y = new(fp.Fp).SetBigInt(y1)
	p.z = new(fp.Fp).SetOne()
	if p.x.IsZero() && p.y.IsZero() {
		p.z.SetZero()
	}

	q := new(Ep)
	q.x = new(fp.Fp).SetBigInt(x2)
	q.y = new(fp.Fp).SetBigInt(y2)
	q.z = new(fp.Fp).SetOne()
	if q.x.IsZero() && q.y.IsZero() {
		q.z.SetZero()
	}
	p.Add(p, q)
	p.toAffine()
	return p.x.BigInt(), p.y.BigInt()
}

func (curve *PallasCurve) Double(x1, y1 *big.Int) (*big.Int, *big.Int) {
	p := new(Ep)
	p.x = new(fp.Fp).SetBigInt(x1)
	p.y = new(fp.Fp).SetBigInt(y1)
	p.z = new(fp.Fp).SetOne()
	if p.x.IsZero() && p.y.IsZero() {
		p.z.SetZero()
	}
	p.Double(p)
	p.toAffine()
	return p.x.BigInt(), p.y.BigInt()
}

func (curve *PallasCurve) ScalarMult(Bx, By *big.Int, k []byte) (*big.Int, *big.Int) {
	p := new(Ep)
	p.x = new(fp.Fp).SetBigInt(Bx)
	p.y = new(fp.Fp).SetBigInt(By)
	p.z = new(fp.Fp).SetOne()
	if p.x.IsZero() && p.y.IsZero() {
		p.z.SetZero()
	}
	var t [32]byte
	copy(t[:], k)
	ss := new(big.Int).SetBytes(k)
	sc := new(fq.Fq).SetBigInt(ss)
	p.Mul(p, sc)
	p.toAffine()
	return p.x.BigInt(), p.y.BigInt()
}

func (curve *PallasCurve) ScalarBaseMult(k []byte) (*big.Int, *big.Int) {
	p := new(Ep).Generator()
	var t [32]byte
	copy(t[:], k)
	ss := new(big.Int).SetBytes(k)
	sc := new(fq.Fq).SetBigInt(ss)
	p.Mul(p, sc)
	p.toAffine()
	return p.x.BigInt(), p.y.BigInt()
}

// PallasScalar - Old interface
type PallasScalar struct{}

func NewPallasScalar() *PallasScalar {
	return &PallasScalar{}
}

func (k PallasScalar) Add(x, y *big.Int) *big.Int {
	r := new(big.Int).Add(x, y)
	return r.Mod(r, fq.BiModulus)
}

func (k PallasScalar) Sub(x, y *big.Int) *big.Int {
	r := new(big.Int).Sub(x, y)
	return r.Mod(r, fq.BiModulus)
}

func (k PallasScalar) Neg(x *big.Int) *big.Int {
	r := new(big.Int).Neg(x)
	return r.Mod(r, fq.BiModulus)
}

func (k PallasScalar) Mul(x, y *big.Int) *big.Int {
	r := new(big.Int).Mul(x, y)
	return r.Mod(r, fq.BiModulus)
}

func (k PallasScalar) Div(x, y *big.Int) *big.Int {
	r := new(big.Int).ModInverse(y, fq.BiModulus)
	r.Mul(r, x)
	return r.Mod(r, fq.BiModulus)
}

func (k PallasScalar) Hash(input []byte) *big.Int {
	return new(ScalarPallas).Hash(input).(*ScalarPallas).value.BigInt()
}

func (k PallasScalar) Bytes(x *big.Int) []byte {
	return x.Bytes()
}

func (k PallasScalar) Random() (*big.Int, error) {
	s, ok := new(ScalarPallas).Random(crand.Reader).(*ScalarPallas)
	if !ok {
		return nil, errors.New("incorrect type conversion")
	}
	return s.value.BigInt(), nil
}

func (k PallasScalar) IsValid(x *big.Int) bool {
	return x.Cmp(fq.BiModulus) == -1
}

// ScalarPallas - New interface
type ScalarPallas struct {
	value *fq.Fq
}

func (s *ScalarPallas) Random(reader io.Reader) Scalar {
	if reader == nil {
		return nil
	}
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return s.Hash(seed[:])
}

func (s *ScalarPallas) Hash(bytes []byte) Scalar {
	h, _ := blake2b.New(64, []byte{})
	xmd, err := expandMsgXmd(h, bytes, []byte("pallas_XMD:BLAKE2b_SSWU_RO_"), 64)
	if err != nil {
		return nil
	}
	var t [64]byte
	copy(t[:], xmd)
	return &ScalarPallas{
		value: new(fq.Fq).SetBytesWide(&t),
	}
}

func (s *ScalarPallas) Zero() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).SetZero(),
	}
}

func (s *ScalarPallas) One() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).SetOne(),
	}
}

func (s *ScalarPallas) IsZero() bool {
	return s.value.IsZero()
}

func (s *ScalarPallas) IsOne() bool {
	return s.value.IsOne()
}

func (s *ScalarPallas) IsOdd() bool {
	return (s.value[0] & 1) == 1
}

func (s *ScalarPallas) IsEven() bool {
	return (s.value[0] & 1) == 0
}

func (s *ScalarPallas) New(value int) Scalar {
	v := big.NewInt(int64(value))
	return &ScalarPallas{
		value: new(fq.Fq).SetBigInt(v),
	}
}

func (s *ScalarPallas) Cmp(rhs Scalar) int {
	r, ok := rhs.(*ScalarPallas)
	if ok {
		return s.value.Cmp(r.value)
	} else {
		return -2
	}
}

func (s *ScalarPallas) Square() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).Square(s.value),
	}
}

func (s *ScalarPallas) Double() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).Double(s.value),
	}
}

func (s *ScalarPallas) Invert() (Scalar, error) {
	value, wasInverted := new(fq.Fq).Invert(s.value)
	if !wasInverted {
		return nil, fmt.Errorf("inverse doesn't exist")
	}
	return &ScalarPallas{
		value,
	}, nil
}

func (s *ScalarPallas) Sqrt() (Scalar, error) {
	value, wasSquare := new(fq.Fq).Sqrt(s.value)
	if !wasSquare {
		return nil, fmt.Errorf("not a square")
	}
	return &ScalarPallas{
		value,
	}, nil
}

func (s *ScalarPallas) Cube() Scalar {
	value := new(fq.Fq).Mul(s.value, s.value)
	value.Mul(value, s.value)
	return &ScalarPallas{
		value,
	}
}

func (s *ScalarPallas) Add(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarPallas)
	if ok {
		return &ScalarPallas{
			value: new(fq.Fq).Add(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarPallas) Sub(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarPallas)
	if ok {
		return &ScalarPallas{
			value: new(fq.Fq).Sub(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarPallas) Mul(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarPallas)
	if ok {
		return &ScalarPallas{
			value: new(fq.Fq).Mul(s.value, r.value),
		}
	} else {
		return nil
	}
}

func (s *ScalarPallas) MulAdd(y, z Scalar) Scalar {
	return s.Mul(y).Add(z)
}

func (s *ScalarPallas) Div(rhs Scalar) Scalar {
	r, ok := rhs.(*ScalarPallas)
	if ok {
		v, wasInverted := new(fq.Fq).Invert(r.value)
		if !wasInverted {
			return nil
		}
		v.Mul(v, s.value)
		return &ScalarPallas{value: v}
	} else {
		return nil
	}
}

func (s *ScalarPallas) Neg() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).Neg(s.value),
	}
}

func (s *ScalarPallas) SetBigInt(v *big.Int) (Scalar, error) {
	return &ScalarPallas{
		value: new(fq.Fq).SetBigInt(v),
	}, nil
}

func (s *ScalarPallas) BigInt() *big.Int {
	return s.value.BigInt()
}

func (s *ScalarPallas) Bytes() []byte {
	t := s.value.Bytes()
	return t[:]
}

func (s *ScalarPallas) SetBytes(bytes []byte) (Scalar, error) {
	if len(bytes) != 32 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [32]byte
	copy(seq[:], bytes)
	value, err := new(fq.Fq).SetBytes(&seq)
	if err != nil {
		return nil, err
	}
	return &ScalarPallas{
		value,
	}, nil
}

func (s *ScalarPallas) SetBytesWide(bytes []byte) (Scalar, error) {
	if len(bytes) != 64 {
		return nil, fmt.Errorf("invalid length")
	}
	var seq [64]byte
	copy(seq[:], bytes)
	return &ScalarPallas{
		value: new(fq.Fq).SetBytesWide(&seq),
	}, nil
}

func (s *ScalarPallas) Point() Point {
	return new(PointPallas).Identity()
}

func (s *ScalarPallas) Clone() Scalar {
	return &ScalarPallas{
		value: new(fq.Fq).Set(s.value),
	}
}

func (s *ScalarPallas) GetFq() *fq.Fq {
	return new(fq.Fq).Set(s.value)
}

func (s *ScalarPallas) SetFq(fq *fq.Fq) *ScalarPallas {
	s.value = fq
	return s
}

func (s *ScalarPallas) MarshalBinary() ([]byte, error) {
	return scalarMarshalBinary(s)
}

func (s *ScalarPallas) UnmarshalBinary(input []byte) error {
	sc, err := scalarUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarPallas)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarPallas) MarshalText() ([]byte, error) {
	return scalarMarshalText(s)
}

func (s *ScalarPallas) UnmarshalText(input []byte) error {
	sc, err := scalarUnmarshalText(input)
	if err != nil {
		return err
	}
	ss, ok := sc.(*ScalarPallas)
	if !ok {
		return fmt.Errorf("invalid scalar")
	}
	s.value = ss.value
	return nil
}

func (s *ScalarPallas) MarshalJSON() ([]byte, error) {
	return scalarMarshalJson(s)
}

func (s *ScalarPallas) UnmarshalJSON(input []byte) error {
	sc, err := scalarUnmarshalJson(input)
	if err != nil {
		return err
	}
	S, ok := sc.(*ScalarPallas)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	s.value = S.value
	return nil
}

type PointPallas struct {
	value *Ep
}

func (p *PointPallas) Random(reader io.Reader) Point {
	return &PointPallas{new(Ep).Random(reader)}
}

func (p *PointPallas) Hash(bytes []byte) Point {
	return &PointPallas{new(Ep).Hash(bytes)}
}

func (p *PointPallas) Identity() Point {
	return &PointPallas{new(Ep).Identity()}
}

func (p *PointPallas) Generator() Point {
	return &PointPallas{new(Ep).Generator()}
}

func (p *PointPallas) IsIdentity() bool {
	return p.value.IsIdentity()
}

func (p *PointPallas) IsNegative() bool {
	return p.value.Y().IsOdd()
}

func (p *PointPallas) IsOnCurve() bool {
	return p.value.IsOnCurve()
}

func (p *PointPallas) Double() Point {
	return &PointPallas{new(Ep).Double(p.value)}
}

func (p *PointPallas) Scalar() Scalar {
	return &ScalarPallas{new(fq.Fq).SetZero()}
}

func (p *PointPallas) Neg() Point {
	return &PointPallas{new(Ep).Neg(p.value)}
}

func (p *PointPallas) Add(rhs Point) Point {
	r, ok := rhs.(*PointPallas)
	if !ok {
		return nil
	}
	return &PointPallas{new(Ep).Add(p.value, r.value)}
}

func (p *PointPallas) Sub(rhs Point) Point {
	r, ok := rhs.(*PointPallas)
	if !ok {
		return nil
	}
	return &PointPallas{new(Ep).Sub(p.value, r.value)}
}

func (p *PointPallas) Mul(rhs Scalar) Point {
	s, ok := rhs.(*ScalarPallas)
	if !ok {
		return nil
	}
	return &PointPallas{new(Ep).Mul(p.value, s.value)}
}

func (p *PointPallas) Equal(rhs Point) bool {
	r, ok := rhs.(*PointPallas)
	if !ok {
		return false
	}
	return p.value.Equal(r.value)
}

func (p *PointPallas) Set(x, y *big.Int) (Point, error) {
	xx := subtle.ConstantTimeCompare(x.Bytes(), []byte{})
	yy := subtle.ConstantTimeCompare(y.Bytes(), []byte{})
	xElem := new(fp.Fp).SetBigInt(x)
	var data [32]byte
	if yy == 1 {
		if xx == 1 {
			return &PointPallas{new(Ep).Identity()}, nil
		}
		data = xElem.Bytes()
		return p.FromAffineCompressed(data[:])
	}
	yElem := new(fp.Fp).SetBigInt(y)
	value := &Ep{xElem, yElem, new(fp.Fp).SetOne()}
	if !value.IsOnCurve() {
		return nil, fmt.Errorf("point is not on the curve")
	}
	return &PointPallas{value}, nil
}

func (p *PointPallas) ToAffineCompressed() []byte {
	return p.value.ToAffineCompressed()
}

func (p *PointPallas) ToAffineUncompressed() []byte {
	return p.value.ToAffineUncompressed()
}

func (p *PointPallas) FromAffineCompressed(bytes []byte) (Point, error) {
	value, err := new(Ep).FromAffineCompressed(bytes)
	if err != nil {
		return nil, err
	}
	return &PointPallas{value}, nil
}

func (p *PointPallas) FromAffineUncompressed(bytes []byte) (Point, error) {
	value, err := new(Ep).FromAffineUncompressed(bytes)
	if err != nil {
		return nil, err
	}
	return &PointPallas{value}, nil
}

func (p *PointPallas) CurveName() string {
	return PallasName
}

func (p *PointPallas) SumOfProducts(points []Point, scalars []Scalar) Point {
	eps := make([]*Ep, len(points))
	for i, pt := range points {
		ps, ok := pt.(*PointPallas)
		if !ok {
			return nil
		}
		eps[i] = ps.value
	}
	value := p.value.SumOfProducts(eps, scalars)
	return &PointPallas{value}
}

func (p *PointPallas) MarshalBinary() ([]byte, error) {
	return pointMarshalBinary(p)
}

func (p *PointPallas) UnmarshalBinary(input []byte) error {
	pt, err := pointUnmarshalBinary(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointPallas)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointPallas) MarshalText() ([]byte, error) {
	return pointMarshalText(p)
}

func (p *PointPallas) UnmarshalText(input []byte) error {
	pt, err := pointUnmarshalText(input)
	if err != nil {
		return err
	}
	ppt, ok := pt.(*PointPallas)
	if !ok {
		return fmt.Errorf("invalid point")
	}
	p.value = ppt.value
	return nil
}

func (p *PointPallas) MarshalJSON() ([]byte, error) {
	return pointMarshalJson(p)
}

func (p *PointPallas) UnmarshalJSON(input []byte) error {
	pt, err := pointUnmarshalJson(input)
	if err != nil {
		return err
	}
	P, ok := pt.(*PointPallas)
	if !ok {
		return fmt.Errorf("invalid type")
	}
	p.value = P.value
	return nil
}

func (p *PointPallas) X() *fp.Fp {
	return p.value.X()
}

func (p *PointPallas) Y() *fp.Fp {
	return p.value.Y()
}

func (p *PointPallas) GetEp() *Ep {
	return new(Ep).Set(p.value)
}

type Ep struct {
	x *fp.Fp
	y *fp.Fp
	z *fp.Fp
}

func (p *Ep) Random(reader io.Reader) *Ep {
	var seed [64]byte
	_, _ = reader.Read(seed[:])
	return p.Hash(seed[:])
}

func (p *Ep) Hash(bytes []byte) *Ep {
	if bytes == nil {
		bytes = []byte{}
	}
	h, _ := blake2b.New(64, []byte{})
	u, _ := expandMsgXmd(h, bytes, []byte("pallas_XMD:BLAKE2b_SSWU_RO_"), 128)
	var buf [64]byte
	copy(buf[:], u[:64])
	u0 := new(fp.Fp).SetBytesWide(&buf)
	copy(buf[:], u[64:])
	u1 := new(fp.Fp).SetBytesWide(&buf)

	q0 := mapSswu(u0)
	q1 := mapSswu(u1)
	r1 := isoMap(q0)
	r2 := isoMap(q1)
	return p.Identity().Add(r1, r2)
}

func (p *Ep) Identity() *Ep {
	p.x = new(fp.Fp).SetZero()
	p.y = new(fp.Fp).SetZero()
	p.z = new(fp.Fp).SetZero()
	return p
}

func (p *Ep) Generator() *Ep {
	p.x = new(fp.Fp).SetOne()
	p.y = &fp.Fp{0x2f474795455d409d, 0xb443b9b74b8255d9, 0x270c412f2c9a5d66, 0x8e00f71ba43dd6b}
	p.z = new(fp.Fp).SetOne()
	return p
}

func (p *Ep) IsIdentity() bool {
	return p.z.IsZero()
}

func (p *Ep) Double(other *Ep) *Ep {
	if other.IsIdentity() {
		p.Set(other)
		return p
	}
	r := new(Ep)
	// essentially paraphrased https://github.com/MinaProtocol/c-reference-signer/blob/master/crypto.c#L306-L337
	a := new(fp.Fp).Square(other.x)
	b := new(fp.Fp).Square(other.y)
	c := new(fp.Fp).Square(b)
	r.x = new(fp.Fp).Add(other.x, b)
	r.y = new(fp.Fp).Square(r.x)
	r.z = new(fp.Fp).Sub(r.y, a)
	r.x.Sub(r.z, c)
	d := new(fp.Fp).Double(r.x)
	e := new(fp.Fp).Mul(three, a)
	f := new(fp.Fp).Square(e)
	r.y.Double(d)
	r.x.Sub(f, r.y)
	r.y.Sub(d, r.x)
	f.Mul(eight, c)
	r.z.Mul(e, r.y)
	r.y.Sub(r.z, f)
	f.Mul(other.y, other.z)
	r.z.Double(f)
	p.Set(r)
	return p
}

func (p *Ep) Neg(other *Ep) *Ep {
	p.x = new(fp.Fp).Set(other.x)
	p.y = new(fp.Fp).Neg(other.y)
	p.z = new(fp.Fp).Set(other.z)
	return p
}

func (p *Ep) Add(lhs *Ep, rhs *Ep) *Ep {
	if lhs.IsIdentity() {
		return p.Set(rhs)
	}
	if rhs.IsIdentity() {
		return p.Set(lhs)
	}
	z1z1 := new(fp.Fp).Square(lhs.z)
	z2z2 := new(fp.Fp).Square(rhs.z)
	u1 := new(fp.Fp).Mul(lhs.x, z2z2)
	u2 := new(fp.Fp).Mul(rhs.x, z1z1)
	s1 := new(fp.Fp).Mul(lhs.y, z2z2)
	s1.Mul(s1, rhs.z)
	s2 := new(fp.Fp).Mul(rhs.y, z1z1)
	s2.Mul(s2, lhs.z)

	if u1.Equal(u2) {
		if s1.Equal(s2) {
			return p.Double(lhs)
		} else {
			return p.Identity()
		}
	} else {
		h := new(fp.Fp).Sub(u2, u1)
		i := new(fp.Fp).Double(h)
		i.Square(i)
		j := new(fp.Fp).Mul(i, h)
		r := new(fp.Fp).Sub(s2, s1)
		r.Double(r)
		v := new(fp.Fp).Mul(u1, i)
		x3 := new(fp.Fp).Square(r)
		x3.Sub(x3, j)
		x3.Sub(x3, new(fp.Fp).Double(v))
		s1.Mul(s1, j)
		s1.Double(s1)
		y3 := new(fp.Fp).Mul(r, new(fp.Fp).Sub(v, x3))
		y3.Sub(y3, s1)
		z3 := new(fp.Fp).Add(lhs.z, rhs.z)
		z3.Square(z3)
		z3.Sub(z3, z1z1)
		z3.Sub(z3, z2z2)
		z3.Mul(z3, h)
		p.x = new(fp.Fp).Set(x3)
		p.y = new(fp.Fp).Set(y3)
		p.z = new(fp.Fp).Set(z3)

		return p
	}
}

func (p *Ep) Sub(lhs, rhs *Ep) *Ep {
	return p.Add(lhs, new(Ep).Neg(rhs))
}

func (p *Ep) Mul(point *Ep, scalar *fq.Fq) *Ep {
	bytes := scalar.Bytes()
	precomputed := [16]*Ep{}
	precomputed[0] = new(Ep).Identity()
	precomputed[1] = new(Ep).Set(point)
	for i := 2; i < 16; i += 2 {
		precomputed[i] = new(Ep).Double(precomputed[i>>1])
		precomputed[i+1] = new(Ep).Add(precomputed[i], point)
	}
	p.Identity()
	for i := 0; i < 256; i += 4 {
		// Brouwer / windowing method. window size of 4.
		for j := 0; j < 4; j++ {
			p.Double(p)
		}
		window := bytes[32-1-i>>3] >> (4 - i&0x04) & 0x0F
		p.Add(p, precomputed[window])
	}
	return p
}

func (p *Ep) Equal(other *Ep) bool {
	// warning: requires converting both to affine
	// could save slightly by modifying one so that its z-value equals the other
	// this would save one inversion and a handful of multiplications
	// but this is more subtle and error-prone, so going to just convert both to affine.
	lhs := new(Ep).Set(p)
	rhs := new(Ep).Set(other)
	lhs.toAffine()
	rhs.toAffine()
	return lhs.x.Equal(rhs.x) && lhs.y.Equal(rhs.y)
}

func (p *Ep) Set(other *Ep) *Ep {
	// check is identity or on curve
	p.x = new(fp.Fp).Set(other.x)
	p.y = new(fp.Fp).Set(other.y)
	p.z = new(fp.Fp).Set(other.z)
	return p
}

func (p *Ep) toAffine() *Ep {
	// mutates `p` in-place to convert it to "affine" form.
	if p.IsIdentity() {
		// warning: control flow / not constant-time
		p.x.SetZero()
		p.y.SetZero()
		p.z.SetOne()
		return p
	}
	zInv3, _ := new(fp.Fp).Invert(p.z) // z is necessarily nonzero
	zInv2 := new(fp.Fp).Square(zInv3)
	zInv3.Mul(zInv3, zInv2)
	p.x.Mul(p.x, zInv2)
	p.y.Mul(p.y, zInv3)
	p.z.SetOne()
	return p
}

func (p *Ep) ToAffineCompressed() []byte {
	// Use ZCash encoding where infinity is all zeros
	// and the top bit represents the sign of y and the
	// remainder represent the x-coordinate
	var inf [32]byte
	p1 := new(Ep).Set(p)
	p1.toAffine()
	x := p1.x.Bytes()
	x[31] |= (p1.y.Bytes()[0] & 1) << 7
	subtle.ConstantTimeCopy(bool2int[p1.IsIdentity()], x[:], inf[:])
	return x[:]
}

func (p *Ep) ToAffineUncompressed() []byte {
	p1 := new(Ep).Set(p)
	p1.toAffine()
	x := p1.x.Bytes()
	y := p1.y.Bytes()
	return append(x[:], y[:]...)
}

func (p *Ep) FromAffineCompressed(bytes []byte) (*Ep, error) {
	if len(bytes) != 32 {
		return nil, fmt.Errorf("invalid byte sequence")
	}

	var input [32]byte
	copy(input[:], bytes)
	sign := (input[31] >> 7) & 1
	input[31] &= 0x7F

	x := new(fp.Fp)
	if _, err := x.SetBytes(&input); err != nil {
		return nil, err
	}
	rhs := rhsPallas(x)
	if _, square := rhs.Sqrt(rhs); !square {
		return nil, fmt.Errorf("rhs of given x-coordinate is not a square")
	}
	if rhs.Bytes()[0]&1 != sign {
		rhs.Neg(rhs)
	}
	p.x = x
	p.y = rhs
	p.z = new(fp.Fp).SetOne()
	if !p.IsOnCurve() {
		return nil, fmt.Errorf("invalid point")
	}
	return p, nil
}

func (p *Ep) FromAffineUncompressed(bytes []byte) (*Ep, error) {
	if len(bytes) != 64 {
		return nil, fmt.Errorf("invalid length")
	}
	p.z = new(fp.Fp).SetOne()
	p.x = new(fp.Fp)
	p.y = new(fp.Fp)
	var x, y [32]byte
	copy(x[:], bytes[:32])
	copy(y[:], bytes[32:])
	if _, err := p.x.SetBytes(&x); err != nil {
		return nil, err
	}
	if _, err := p.y.SetBytes(&y); err != nil {
		return nil, err
	}
	if !p.IsOnCurve() {
		return nil, fmt.Errorf("invalid point")
	}
	return p, nil
}

// rhs of the curve equation
func rhsPallas(x *fp.Fp) *fp.Fp {
	x2 := new(fp.Fp).Square(x)
	x3 := new(fp.Fp).Mul(x, x2)
	return new(fp.Fp).Add(x3, b)
}

func (p Ep) CurveName() string {
	return "pallas"
}

func (p Ep) SumOfProducts(points []*Ep, scalars []Scalar) *Ep {
	nScalars := make([]*big.Int, len(scalars))
	for i, s := range scalars {
		sc, ok := s.(*ScalarPallas)
		if !ok {
			return nil
		}
		nScalars[i] = sc.value.BigInt()
	}
	return sumOfProductsPippengerPallas(points, nScalars)
}

func (p *Ep) X() *fp.Fp {
	t := new(Ep).Set(p)
	t.toAffine()
	return new(fp.Fp).Set(t.x)
}

func (p *Ep) Y() *fp.Fp {
	t := new(Ep).Set(p)
	t.toAffine()
	return new(fp.Fp).Set(t.y)
}

func (p *Ep) IsOnCurve() bool {
	// y^2 = x^3 + axz^4 + bz^6
	// a = 0
	// b = 5
	z2 := new(fp.Fp).Square(p.z)
	z4 := new(fp.Fp).Square(z2)
	z6 := new(fp.Fp).Mul(z2, z4)
	x2 := new(fp.Fp).Square(p.x)
	x3 := new(fp.Fp).Mul(x2, p.x)

	lhs := new(fp.Fp).Square(p.y)
	rhs := new(fp.Fp).SetUint64(5)
	rhs.Mul(rhs, z6)
	rhs.Add(rhs, x3)
	return p.z.IsZero() || lhs.Equal(rhs)
}

func (p *Ep) CMove(lhs, rhs *Ep, condition int) *Ep {
	p.x = new(fp.Fp).CMove(lhs.x, rhs.x, condition)
	p.y = new(fp.Fp).CMove(lhs.y, rhs.y, condition)
	p.z = new(fp.Fp).CMove(lhs.z, rhs.z, condition)
	return p
}

func sumOfProductsPippengerPallas(points []*Ep, scalars []*big.Int) *Ep {
	if len(points) != len(scalars) {
		return nil
	}

	const w = 6

	bucketSize := (1 << w) - 1
	windows := make([]*Ep, 255/w+1)
	for i := range windows {
		windows[i] = new(Ep).Identity()
	}
	bucket := make([]*Ep, bucketSize)

	for j := 0; j < len(windows); j++ {
		for i := 0; i < bucketSize; i++ {
			bucket[i] = new(Ep).Identity()
		}

		for i := 0; i < len(scalars); i++ {
			index := bucketSize & int(new(big.Int).Rsh(scalars[i], uint(w*j)).Int64())
			if index != 0 {
				bucket[index-1].Add(bucket[index-1], points[i])
			}
		}

		acc, sum := new(Ep).Identity(), new(Ep).Identity()

		for i := bucketSize - 1; i >= 0; i-- {
			sum.Add(sum, bucket[i])
			acc.Add(acc, sum)
		}
		windows[j] = acc
	}

	acc := new(Ep).Identity()
	for i := len(windows) - 1; i >= 0; i-- {
		for j := 0; j < w; j++ {
			acc.Double(acc)
		}
		acc.Add(acc, windows[i])
	}
	return acc
}

// Implements a degree 3 isogeny map.
// The input and output are in Jacobian coordinates, using the method
// in "Avoiding inversions" [WB2019, section 4.3].
func isoMap(p *Ep) *Ep {
	var z [4]*fp.Fp
	z[0] = new(fp.Fp).Square(p.z)    //z^2
	z[1] = new(fp.Fp).Mul(z[0], p.z) // z^3
	z[2] = new(fp.Fp).Square(z[0])   // z^4
	z[3] = new(fp.Fp).Square(z[1])   // z^6

	// ((iso[0] * x + iso[1] * z^2) * x + iso[2] * z^4) * x + iso[3] * z^6
	numX := new(fp.Fp).Set(isomapper[0])
	numX.Mul(numX, p.x)
	numX.Add(numX, new(fp.Fp).Mul(isomapper[1], z[0]))
	numX.Mul(numX, p.x)
	numX.Add(numX, new(fp.Fp).Mul(isomapper[2], z[2]))
	numX.Mul(numX, p.x)
	numX.Add(numX, new(fp.Fp).Mul(isomapper[3], z[3]))

	// (z^2 * x + iso[4] * z^4) * x + iso[5] * z^6
	divX := new(fp.Fp).Set(z[0])
	divX.Mul(divX, p.x)
	divX.Add(divX, new(fp.Fp).Mul(isomapper[4], z[2]))
	divX.Mul(divX, p.x)
	divX.Add(divX, new(fp.Fp).Mul(isomapper[5], z[3]))

	// (((iso[6] * x + iso[7] * z2) * x + iso[8] * z4) * x + iso[9] * z6) * y
	numY := new(fp.Fp).Set(isomapper[6])
	numY.Mul(numY, p.x)
	numY.Add(numY, new(fp.Fp).Mul(isomapper[7], z[0]))
	numY.Mul(numY, p.x)
	numY.Add(numY, new(fp.Fp).Mul(isomapper[8], z[2]))
	numY.Mul(numY, p.x)
	numY.Add(numY, new(fp.Fp).Mul(isomapper[9], z[3]))
	numY.Mul(numY, p.y)

	// (((x + iso[10] * z2) * x + iso[11] * z4) * x + iso[12] * z6) * z3
	divY := new(fp.Fp).Set(p.x)
	divY.Add(divY, new(fp.Fp).Mul(isomapper[10], z[0]))
	divY.Mul(divY, p.x)
	divY.Add(divY, new(fp.Fp).Mul(isomapper[11], z[2]))
	divY.Mul(divY, p.x)
	divY.Add(divY, new(fp.Fp).Mul(isomapper[12], z[3]))
	divY.Mul(divY, z[1])

	z0 := new(fp.Fp).Mul(divX, divY)
	x := new(fp.Fp).Mul(numX, divY)
	x.Mul(x, z0)
	y := new(fp.Fp).Mul(numY, divX)
	y.Mul(y, new(fp.Fp).Square(z0))

	return &Ep{
		x, y, z0,
	}
}

func mapSswu(u *fp.Fp) *Ep {
	//c1 := new(fp.Fp).Neg(isoa)
	//c1.Invert(c1)
	//c1.Mul(isob, c1)
	c1 := &fp.Fp{
		0x1ee770ce078456ec,
		0x48cfd64c2ce76be0,
		0x43d5774c0ab79e2f,
		0x23368d2bdce28cf3,
	}
	//c2 := new(fp.Fp).Neg(z)
	//c2.Invert(c2)
	c2 := &fp.Fp{
		0x03df915f89d89d8a,
		0x8f1e8db09ef82653,
		0xd89d89d89d89d89d,
		0x1d89d89d89d89d89,
	}

	u2 := new(fp.Fp).Square(u)
	tv1 := new(fp.Fp).Mul(z, u2)
	tv2 := new(fp.Fp).Square(tv1)
	x1 := new(fp.Fp).Add(tv1, tv2)
	x1.Invert(x1)
	e1 := bool2int[x1.IsZero()]
	x1.Add(x1, new(fp.Fp).SetOne())
	x1.CMove(x1, c2, e1)
	x1.Mul(x1, c1)
	gx1 := new(fp.Fp).Square(x1)
	gx1.Add(gx1, isoa)
	gx1.Mul(gx1, x1)
	gx1.Add(gx1, isob)
	x2 := new(fp.Fp).Mul(tv1, x1)
	tv2.Mul(tv1, tv2)
	gx2 := new(fp.Fp).Mul(gx1, tv2)
	gx1Sqrt, e2 := new(fp.Fp).Sqrt(gx1)
	x := new(fp.Fp).CMove(x2, x1, bool2int[e2])
	gx2Sqrt, _ := new(fp.Fp).Sqrt(gx2)
	y := new(fp.Fp).CMove(gx2Sqrt, gx1Sqrt, bool2int[e2])
	e3 := u.IsOdd() == y.IsOdd()
	y.CMove(new(fp.Fp).Neg(y), y, bool2int[e3])

	return &Ep{
		x: x, y: y, z: new(fp.Fp).SetOne(),
	}
}
