//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

// Package curves: Field implementation IS NOT constant time as it leverages math/big for big number operations.
package curves

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sync"
)

var ed25519SubGroupOrderOnce sync.Once
var ed25519SubGroupOrder *big.Int

// Field is a finite field.
type Field struct {
	*big.Int
}

// Element is a group element within a finite field.
type Element struct {
	Modulus *Field   `json:"modulus"`
	Value   *big.Int `json:"value"`
}

// ElementJSON is used in JSON<>Element conversions.
// For years, big.Int hasn't properly supported JSON unmarshaling
// https://github.com/golang/go/issues/28154
type ElementJSON struct {
	Modulus string `json:"modulus"`
	Value   string `json:"value"`
}

// Marshal Element to JSON
func (x *Element) MarshalJSON() ([]byte, error) {
	return json.Marshal(ElementJSON{
		Modulus: x.Modulus.String(),
		Value:   x.Value.String(),
	})
}

func (x *Element) UnmarshalJSON(bytes []byte) error {
	var e ElementJSON
	err := json.Unmarshal(bytes, &e)
	if err != nil {
		return err
	}
	// Convert the strings to big.Ints
	modulus, ok := new(big.Int).SetString(e.Modulus, 10)
	if !ok {
		return fmt.Errorf("failed to unmarshal modulus string '%v' to big.Int", e.Modulus)
	}
	x.Modulus = &Field{modulus}
	x.Value, ok = new(big.Int).SetString(e.Value, 10)
	if !ok {
		return fmt.Errorf("failed to unmarshal value string '%v' to big.Int", e.Value)
	}
	return nil
}

// The probability of returning true for a randomly chosen
// non-prime is at most ¼ⁿ. 64 is a widely used standard
// that is more than sufficient.
const millerRabinRounds = 64

// New is a constructor for a Field.
func NewField(modulus *big.Int) *Field {
	// For our purposes we never expect to be dealing with a non-prime field. This provides some protection against
	// accidentally doing that.
	if !modulus.ProbablyPrime(millerRabinRounds) {
		panic(fmt.Sprintf("modulus: %x is not a prime", modulus))
	}

	return &Field{modulus}
}

func newElement(field *Field, value *big.Int) *Element {
	if !field.IsValid(value) {
		panic(fmt.Sprintf("value: %x is not within field: %x", value, field))
	}

	return &Element{field, value}
}

// IsValid returns whether or not the value is within [0, modulus)
func (f Field) IsValid(value *big.Int) bool {
	// value < modulus && value >= 0
	return value.Cmp(f.Int) < 0 && value.Sign() >= 0
}

func (f Field) NewElement(value *big.Int) *Element {
	return newElement(&f, value)
}

func (f Field) Zero() *Element {
	return newElement(&f, big.NewInt(0))
}

func (f Field) One() *Element {
	return newElement(&f, big.NewInt(1))
}

func (f Field) RandomElement(r io.Reader) (*Element, error) {
	if r == nil {
		r = rand.Reader
	}
	var randInt *big.Int
	var err error
	// Ed25519 needs to do special handling
	// in case the value is used in
	// Scalar multiplications with points
	if f.Int.Cmp(Ed25519Order()) == 0 {
		scalar := NewEd25519Scalar()
		randInt, err = scalar.RandomWithReader(r)
	} else {
		// Read a random integer within the field. This is defined as [0, max) so we don't need to
		// explicitly check it is within the field. If it is not, NewElement will panic anyways.
		randInt, err = rand.Int(r, f.Int)
	}
	if err != nil {
		return nil, err
	}
	return newElement(&f, randInt), nil
}

// ElementFromBytes initializes a new field element from big-endian bytes
func (f Field) ElementFromBytes(bytes []byte) *Element {
	return newElement(&f, new(big.Int).SetBytes(bytes))
}

// ReducedElementFromBytes initializes a new field element from big-endian bytes and reduces it by
// the modulus of the field.
//
// WARNING: If this is used with cryptographic constructions which rely on a uniform distribution of
// values, this may introduce a bias to the value of the returned field element. This happens when
// the integer range of the provided bytes is not an integer multiple of the field order.
//
// Assume we are working in field which a modulus of 3 and the range of the uniform random bytes we
// provide as input is 5. Thus, the set of field elements is {0, 1, 2} and the set of integer values
// for the input bytes is: {0, 1, 2, 3, 4}. What is the distribution of the output values produced
// by this function?
//
//   ReducedElementFromBytes(0) => 0
//   ReducedElementFromBytes(1) => 1
//   ReducedElementFromBytes(2) => 2
//   ReducedElementFromBytes(3) => 0
//   ReducedElementFromBytes(4) => 1
//
// For a value space V and random value v, a uniform distribution is defined as P[V = v] = 1/|V|
// where |V| is to the order of the field. Using the results from above, we see that P[v = 0] = 2/5,
// P[v = 1] = 2/5, and P[v = 2] = 1/5. For a uniform distribution we would expect these to each be
// equal to 1/3. As they do not, this does not return uniform output for that example.
//
// To see why this is okay if the range is a multiple of the field order, change the input range to
// 6 and notice that now each output has a probability of 2/6 = 1/3, and the output is uniform.
func (f Field) ReducedElementFromBytes(bytes []byte) *Element {
	value := new(big.Int).SetBytes(bytes)
	value.Mod(value, f.Int)
	return newElement(&f, value)
}

func (x Element) Field() *Field {
	return x.Modulus
}

// Add returns the sum x+y
func (x Element) Add(y *Element) *Element {
	x.validateFields(y)

	sum := new(big.Int).Add(x.Value, y.Value)
	sum.Mod(sum, x.Modulus.Int)
	return newElement(x.Modulus, sum)
}

// Sub returns the difference x-y
func (x Element) Sub(y *Element) *Element {
	x.validateFields(y)

	difference := new(big.Int).Sub(x.Value, y.Value)
	difference.Mod(difference, x.Modulus.Int)
	return newElement(x.Modulus, difference)
}

// Neg returns the field negation
func (x Element) Neg() *Element {
	z := new(big.Int).Neg(x.Value)
	z.Mod(z, x.Modulus.Int)
	return newElement(x.Modulus, z)
}

// Mul returns the product x*y
func (x Element) Mul(y *Element) *Element {
	x.validateFields(y)

	product := new(big.Int).Mul(x.Value, y.Value)
	product.Mod(product, x.Modulus.Int)
	return newElement(x.Modulus, product)
}

// Div returns the quotient x/y
func (x Element) Div(y *Element) *Element {
	x.validateFields(y)

	yInv := new(big.Int).ModInverse(y.Value, x.Modulus.Int)
	quotient := new(big.Int).Mul(x.Value, yInv)
	quotient.Mod(quotient, x.Modulus.Int)
	return newElement(x.Modulus, quotient)
}

// Pow computes x^y reduced by the modulus
func (x Element) Pow(y *Element) *Element {
	x.validateFields(y)

	return newElement(x.Modulus, new(big.Int).Exp(x.Value, y.Value, x.Modulus.Int))
}

func (x Element) Invert() *Element {
	return newElement(x.Modulus, new(big.Int).ModInverse(x.Value, x.Modulus.Int))
}

func (x Element) Sqrt() *Element {
	return newElement(x.Modulus, new(big.Int).ModSqrt(x.Value, x.Modulus.Int))
}

// BigInt returns value as a big.Int
func (x Element) BigInt() *big.Int {
	return x.Value
}

// Bytes returns the value as bytes
func (x Element) Bytes() []byte {
	return x.BigInt().Bytes()
}

// IsEqual returns x == y
func (x Element) IsEqual(y *Element) bool {
	if !x.isEqualFields(y) {
		return false
	}

	return x.Value.Cmp(y.Value) == 0
}

// Clone returns a new copy of the element
func (x Element) Clone() *Element {
	return x.Modulus.ElementFromBytes(x.Bytes())
}

func (x Element) isEqualFields(y *Element) bool {
	return x.Modulus.Int.Cmp(y.Modulus.Int) == 0
}

func (x Element) validateFields(y *Element) {
	if !x.isEqualFields(y) {
		panic("fields must match for valid binary operation")
	}
}

// SubgroupOrder returns the order of the Ed25519 base Point.
func Ed25519Order() *big.Int {
	ed25519SubGroupOrderOnce.Do(func() {
		order, ok := new(big.Int).SetString(
			"1000000000000000000000000000000014DEF9DEA2F79CD65812631A5CF5D3ED",
			16,
		)
		if !ok {
			panic("invalid hex string provided. This should never happen as it is constant.")
		}
		ed25519SubGroupOrder = order
	})

	return ed25519SubGroupOrder
}
