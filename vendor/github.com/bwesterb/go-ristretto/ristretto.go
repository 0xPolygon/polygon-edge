// Pure Go implementation of the Ristretto prime-order group built from
// the Edwards curve Edwards25519.
//
// Many cryptographic schemes need a group of prime order.  Popular and
// efficient elliptic curves like (Edwards25519 of `ed25519` fame) are
// rarely of prime order.  There is, however, a convenient method
// to construct a prime order group from such curves, using a method
// called Ristretto proposed by Mike Hamburg.
//
// This package implements the Ristretto group constructed from Edwards25519.
// The Point type represents a group element.  The API mimics that of the
// math/big package.  For instance, to set c to a+b, one writes
//
//     var c ristretto.Point
//     c.Add(&a, &b) // sets c to a + b
//
// Warning: contrary to math.Big's interface, an uninitialized  Point is not
// the same thing as the zero (neutral element) of the group:
//
//     var c ristretto.Point // c is uninitialized now --- not zero!
//     c.SetZero() // c is zero now; ready to use!
//
// Most methods return the receiver, so that function can be chained:
//
//     s.Add(&a, &b).Add(&s, &c)  // sets s to a + b + c
//
// The order of the Ristretto group is l =
// 2^252 + 27742317777372353535851937790883648493 =
// 7237005577332262213973186563042994240857116359379907606001950938285454250989.
// The Scalar type implement the numbers modulo l and also has an API similar
// to math/big.
package ristretto

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/bwesterb/go-ristretto/edwards25519"
)

// Represents an element of the Ristretto group over Edwards25519.
//
// Warning: an uninitialized Point is not the same thing as a zero.  Use
// the SetZero() method to set an (uninitialized) Point to zero.
type Point edwards25519.ExtendedPoint

// A table to speed up scalar multiplication of a fixed point
type ScalarMultTable edwards25519.ScalarMultTable

// Sets p to zero (the neutral element).  Returns p.
func (p *Point) SetZero() *Point {
	p.e().SetZero()
	return p
}

// Sets p to the Edwards25519 basepoint.  Returns p
func (p *Point) SetBase() *Point {
	p.e().SetBase()
	return p
}

// Sets p to q.  Returns p
func (p *Point) Set(q *Point) *Point {
	p.e().Set(q.e())
	return p
}

// Sets p to q + r.  Returns p.
func (p *Point) Add(q, r *Point) *Point {
	p.e().Add(q.e(), r.e())
	return p
}

// Sets p to q + q.  Returns p.
func (p *Point) Double(q *Point) *Point {
	p.e().Double(q.e())
	return p
}

// Sets p to q - r.  Returns p.
func (p *Point) Sub(q, r *Point) *Point {
	p.e().Sub(q.e(), r.e())
	return p
}

// Sets p to -q.  Returns p.
func (p *Point) Neg(q *Point) *Point {
	p.e().Neg(q.e())
	return p
}

// Packs p into the given buffer.  Returns p.
func (p *Point) BytesInto(buf *[32]byte) *Point {
	p.e().RistrettoInto(buf)
	return p
}

// Returns a packed version of p.
func (p *Point) Bytes() []byte {
	return p.e().Ristretto()
}

// Sets p to the point encoded in buf using Bytes().
// Not every input encodes a point.  Returns whether the buffer encoded a point.
func (p *Point) SetBytes(buf *[32]byte) bool {
	return p.e().SetRistretto(buf)
}

// Sets p to the point corresponding to buf using the Elligator2 encoding.
//
// In contrast to SetBytes() (1) Every input buffer will decode to a point
// and (2) SetElligator() is not injective: for every point there are
// approximately four buffers that will encode to it.
func (p *Point) SetElligator(buf *[32]byte) *Point {
	var fe edwards25519.FieldElement
	var cp edwards25519.CompletedPoint
	fe.SetBytes(buf)
	cp.SetRistrettoElligator2(&fe)
	p.e().SetCompleted(&cp)
	return p
}

// Sets p to s * q, where q is the point for which the table t was
// computed. Returns p.
func (p *Point) ScalarMultTable(t *ScalarMultTable, s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	t.t().ScalarMult(p.e(), &buf)
	return p
}

// Sets p to s * q, where q is the point for which the table t was
// computed. Returns p.
//
// Warning: this method uses a non-constant time inmplementation and thus leaks
// information about s.  Use this function only if s is public knowledge.
func (p *Point) PublicScalarMultTable(t *ScalarMultTable, s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	t.t().VarTimeScalarMult(p.e(), &buf)
	return p
}

// Sets p to s * q.  Returns p.
func (p *Point) ScalarMult(q *Point, s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	p.e().ScalarMult(q.e(), &buf)
	return p
}

// Sets p to s * q assuming s is *not* secret.  Returns p.
//
// Warning: this method uses a non-constant time inmplementation and thus leaks
// information about s.  Use this function only if s is public knowledge.
func (p *Point) PublicScalarMult(q *Point, s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	p.e().VarTimeScalarMult(q.e(), &buf)
	return p
}

// Sets p to s * B, where B is the edwards25519 basepoint. Returns p.
//
// Warning: this method uses a non-constant time inmplementation and thus leaks
// information about s.  Use this function only if s is public knowledge.
func (p *Point) PublicScalarMultBase(s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	edwards25519.BaseScalarMultTable.VarTimeScalarMult(p.e(), &buf)
	return p
}

// Sets p to s * B, where B is the edwards25519 basepoint. Returns p.
func (p *Point) ScalarMultBase(s *Scalar) *Point {
	var buf [32]byte
	s.BytesInto(&buf)
	edwards25519.BaseScalarMultTable.ScalarMult(p.e(), &buf)
	return p
}

// Sets p to a random point.  Returns p.
func (p *Point) Rand() *Point {
	var buf [32]byte
	rand.Read(buf[:])
	return p.SetElligator(&buf)
}

// Sets p to the point derived from the buffer using SHA512 and Elligator2.
// Returns p.
//
// NOTE curve25519-dalek uses a different (more conservative) method to derive
// a point from raw data with a hash.  This is implemented in
// Point.DeriveDalek().
func (p *Point) Derive(buf []byte) *Point {
	var ptBuf [32]byte
	h := sha512.Sum512(buf)
	copy(ptBuf[:], h[:32])
	return p.SetElligator(&ptBuf)
}

// Encode 16 bytes into a point using the Lizard method.
//
// Use Lizard() or LizardInto() to decode the bytes from a Point.
//
// Notes on usage:
//
//  - If you want to create a Point from random data, you should rather
//    create a random Point with Point.Rand() and then use (a hash of)
//    Point.Bytes() as the random data.
//  - If you want to derive a Point from data, but you do not care about
//    decoding the data back from the point, you should use
//    the Point.Derive() method instead.
//  - There are some (and with high probability at most 80) inputs to
//    SetLizard() which cannot be decoded.  The chance that you hit such
//    an input is around 1 in 2^122.
//
//    In Lizard there are 256 - 128 - 3 = 125 check bits to pick out the
//    right preimage among at most eight.  Conservatively assuming there are
//    seven other preimages, the chance that one of them passes the check as
//    well is given by:
//
//      1 - (1 - 2^-125)^7 =  7*2^-125 + 21*2^-250 - ...
//                         =~ 2^(-125 - 2log(7))
//                         =  2^-122.192...
//
//    Presuming a random hash function, the number of "bad" inputs is binomially
//    distributed with n=2^128 and p=2^-122.192... For such large n, the Poisson
//    distribution with lambda=n*p=56 is a  very good approximation.  In fact:
//    the cumulative distribution function (CDF) of the Poission distribution
//    is larger than that of the binomial distribution for k > lambda.[1]  The value
//    of the former on k=80 is larger than 0.999 and so with a probability of 99.9%,
//    there are fewer than 80 bad inputs.
//
//  [1] See "Some Inequalities Among Binomial and Poisson Probabilities"
//      by Anderson and Samuels in Proc. Fifth Berkeley Symp. on
//      Math. Statist. and Prob., Vol. 1 (Univ. of Calif. Press, 1967).
func (p *Point) SetLizard(data *[16]byte) *Point {
	var fe edwards25519.FieldElement
	var cp edwards25519.CompletedPoint
	buf := sha256.Sum256(data[:])
	copy(buf[8:], data[:])
	buf[0] &= 254 // clear lowest bit to make the FieldElement positive
	buf[31] &= 63 // clear highest two bits to ensure below 2^255-19.
	fe.SetBytes(&buf)
	cp.SetRistrettoElligator2(&fe)
	p.e().SetCompleted(&cp)
	return p
}

// Decodes 16 bytes encoded into this point using SetLizard().
//
// Returns nil if this point does not contain data encoded using Lizard.
//
// See SetLizard() for notes on usage.
func (p *Point) Lizard() []byte {
	var ret [16]byte
	if p.LizardInto(&ret) != nil {
		return nil
	}
	return ret[:]
}

// Decodes 16 bytes into the given buffer  encoded into this point
// using SetLizard().
//
// See SetLizard() for notes on usage.
func (p *Point) LizardInto(buf *[16]byte) error {
	var fes [8]edwards25519.FieldElement
	var buf2 [32]byte
	var nFound uint8

	mask := p.e().RistrettoElligator2Inverse(&fes)

	for j := 0; j < 8; j++ {
		ok := (mask >> uint(j)) & 1
		fes[j].BytesInto(&buf2)
		h := sha256.Sum256(buf2[8:24])
		copy(h[8:], buf2[8:24])
		h[0] &= 254
		h[31] &= 63
		ok &= uint8(subtle.ConstantTimeCompare(h[:], buf2[:]))
		subtle.ConstantTimeCopy(int(ok), buf[:], buf2[8:24])
		nFound += ok
	}
	if nFound == 1 {
		return nil
	}
	if nFound == 0 {
		return errors.New("No Lizard preimage")
	}
	return errors.New("Multiple Lizard preimages")
}

// Returns 1 if p == q and 0 otherwise.
func (p *Point) EqualsI(q *Point) int32 {
	return p.e().RistrettoEqualsI(q.e())
}

// Returns whether p == q
func (p *Point) Equals(q *Point) bool {
	return p.EqualsI(q) == 1
}

// Sets p to the point derived from the buffer using SHA512 and Elligator2
// in the fashion of curve25519-dalek.
//
// NOTE See also Derive(), which is a different method which is twice as fast,
// but which might not be as secure as this method.
func (p *Point) DeriveDalek(data []byte) *Point {
	hash := sha512.Sum512(data)
	var p2 Point
	var buf [32]byte
	copy(buf[:], hash[:32])
	p.SetElligator(&buf)
	copy(buf[:], hash[32:])
	p2.SetElligator(&buf)
	p.Add(p, &p2)
	return p
}

// Implements encoding/BinaryUnmarshaler. Use SetBytes, if convenient, instead.
func (p *Point) UnmarshalBinary(data []byte) error {
	if len(data) != 32 {
		return fmt.Errorf("ristretto.Point should be 32 bytes; not %d", len(data))
	}
	var buf [32]byte
	copy(buf[:], data)
	if !p.SetBytes(&buf) {
		return errors.New("Buffer does not encode a ristretto.Point")
	}
	return nil
}

// Implements encoding/BinaryMarshaler. Use BytesInto, if convenient, instead.
func (p *Point) MarshalBinary() ([]byte, error) {
	var buf [32]byte
	p.BytesInto(&buf)
	return buf[:], nil
}

func (p *Point) MarshalText() ([]byte, error) {
	enc := base64.RawURLEncoding
	var buf [32]byte
	p.BytesInto(&buf)
	ret := make([]byte, enc.EncodedLen(32))
	enc.Encode(ret, buf[:])
	return ret, nil
}

func textToBuf(dst, src []byte) error {
	var n int
	var err error
	if len(src) == 64 {
		n, err = hex.Decode(dst, src)
		if n == 32 && err == nil {
			return nil
		}
	}
	enc := base64.RawURLEncoding
	n, err = enc.Decode(dst, src)
	if err != nil {
		return err
	}
	if n != 32 {
		return fmt.Errorf("ristretto.Point should be 32 bytes; not %d", n)
	}
	return nil
}

func (p *Point) UnmarshalText(txt []byte) error {
	var buf [32]byte
	err := textToBuf(buf[:], txt)
	if err != nil {
		return err
	}
	if !p.SetBytes(&buf) {
		return errors.New("Buffer does not encode a ristretto.Point")
	}
	return nil
}

func (p Point) String() string {
	text, _ := p.MarshalText()
	return string(text)
}

func (p *Point) e() *edwards25519.ExtendedPoint {
	return (*edwards25519.ExtendedPoint)(p)
}

func (t *ScalarMultTable) t() *edwards25519.ScalarMultTable {
	return (*edwards25519.ScalarMultTable)(t)
}

// Fills the table for point p.
func (t *ScalarMultTable) Compute(p *Point) {
	t.t().Compute(p.e())
}
