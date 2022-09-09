//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//
// Package core contains convenience functions for modular arithmetic.

// Package core contains a set of primitives, including but not limited to various
// elliptic curves, hashes, and commitment schemes. These primitives are used internally
// and can also be used independently on their own externally.
package core

import (
	crand "crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"

	"github.com/coinbase/kryptology/internal"
)

var (
	// Zero is additive identity in the set of integers
	Zero = big.NewInt(0)

	// One is the multiplicative identity in the set of integers
	One = big.NewInt(1)

	// Two is the odd prime
	Two = big.NewInt(2)
)

// ConstantTimeEqByte determines if a, b have identical byte serialization
// and signs. It uses the crypto/subtle package to get a constant time comparison
// over byte representations. Return value is a byte which may be
// useful in bitwise operations. Returns 0x1 if the two values have the
// identical sign and byte representation; 0x0 otherwise.
func ConstantTimeEqByte(a, b *big.Int) byte {
	if a == nil && a == b {
		return 1
	}
	if a == nil || b == nil {
		return 0
	}
	// Determine if the byte representations are the same
	var sameBytes byte
	if subtle.ConstantTimeCompare(a.Bytes(), b.Bytes()) == 1 {
		sameBytes = 1
	} else {
		sameBytes = 0
	}

	// Determine if the signs are the same
	var sameSign byte
	if a.Sign() == b.Sign() {
		sameSign = 1
	} else {
		sameSign = 0
	}

	// Report the conjunction
	return sameBytes & sameSign
}

// ConstantTimeEq determines if a, b have identical byte serialization
// and uses the crypto/subtle package to get a constant time comparison
// over byte representations.
func ConstantTimeEq(a, b *big.Int) bool {
	return ConstantTimeEqByte(a, b) == 1
}

// In determines ring membership before modular reduction: x ∈ Z_m
// returns nil if 0 ≤ x < m
func In(x, m *big.Int) error {
	if AnyNil(x, m) {
		return internal.ErrNilArguments
	}
	// subtle doesn't support constant time big.Int compare
	// just use big.Cmp for now
	// x ∈ Z_m ⇔ 0 ≤ x < m
	if x.Cmp(Zero) != -1 && x.Cmp(m) == -1 {
		return nil
	}
	return internal.ErrZmMembership
}

// Add (modular addition): z = x+y (modulo m)
func Add(x, y, m *big.Int) (*big.Int, error) {
	if AnyNil(x, y) {
		return nil, internal.ErrNilArguments
	}
	z := new(big.Int).Add(x, y)
	// Compute the residue if one is specified, otherwise
	// we leave the value as an unbound integer
	if m != nil {
		z.Mod(z, m)
	}
	return z, nil
}

// Mul (modular multiplication): z = x*y (modulo m)
func Mul(x, y, m *big.Int) (*big.Int, error) {
	if AnyNil(x, y) {
		return nil, internal.ErrNilArguments
	}
	z := new(big.Int).Mul(x, y)

	// Compute the residue if one is specified, otherwise
	// we leave the value as an unbound integer
	if m != nil {
		z.Mod(z, m)
	}
	return z, nil
}

// Exp (modular exponentiation): z = x^y (modulo m)
func Exp(x, y, m *big.Int) (*big.Int, error) {
	if AnyNil(x, y) {
		return nil, internal.ErrNilArguments
	}
	// This wrapper looks silly, but it makes the calling code read more consistently.
	return new(big.Int).Exp(x, y, m), nil
}

// Neg (modular negation): z = -x (modulo m)
func Neg(x, m *big.Int) (*big.Int, error) {
	if AnyNil(x, m) {
		return nil, internal.ErrNilArguments
	}
	z := new(big.Int).Neg(x)
	z.Mod(z, m)
	return z, nil
}

// Inv (modular inverse): returns y such that xy = 1 (modulo m).
func Inv(x, m *big.Int) (*big.Int, error) {
	if AnyNil(x, m) {
		return nil, internal.ErrNilArguments
	}
	z := new(big.Int).ModInverse(x, m)
	if z == nil {
		return nil, fmt.Errorf("cannot compute the multiplicative inverse")
	}
	return z, nil
}

// Rand generates a cryptographically secure random integer in the range: 1 < r < m.
func Rand(m *big.Int) (*big.Int, error) {
	if m == nil {
		return nil, internal.ErrNilArguments
	}

	// Select a random element, but not zero or one
	// The reason is the random element may be used as a Scalar or an exponent.
	// An exponent of 1 is generally acceptable because the generator can't be
	// 1. If a Scalar is combined with another Scalar like in fiat-shamir, it
	// offers no hiding properties when multiplied.
	for {
		result, err := crand.Int(crand.Reader, m)
		if err != nil {
			return nil, err
		}

		if result.Cmp(One) == 1 { // result > 1
			return result, nil
		}
	}
}

// AnyNil determines if any of values are nil
func AnyNil(values ...*big.Int) bool {
	for _, x := range values {
		if x == nil {
			return true
		}
	}
	return false
}
