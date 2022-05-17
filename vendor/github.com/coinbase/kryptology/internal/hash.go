//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package internal

import (
	"bytes"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/hkdf"
)

// Hash computes the HKDF over many values
// iteratively such that each value is hashed separately
// and based on preceding values
//
// The first value is computed as okm_0 = KDF(f || value) where
// f is a byte slice of 32 0xFF
// salt is zero-filled byte slice with length equal to the hash output length
// info is the protocol name
// okm is the 32 byte output
//
// The each subsequent iteration is computed by as okm_i = KDF(f_i || value || okm_{i-1})
// where f_i = 2^b - 1 - i such that there are 0xFF bytes prior to the value.
// f_1 changes the first byte to 0xFE, f_2 to 0xFD. The previous okm is appended to the value
// to provide cryptographic domain separation.
// See https://signal.org/docs/specifications/x3dh/#cryptographic-notation
// and https://signal.org/docs/specifications/xeddsa/#hash-functions
// for more details.
// This uses the KDF function similar to X3DH for each `value`
// But changes the key just like XEdDSA where the prefix bytes change by a single bit
func Hash(info []byte, values ...[]byte) ([]byte, error) {
	// Don't accept any nil arguments
	if anyNil(values...) {
		return nil, ErrNilArguments
	}

	salt := make([]byte, 32)
	okm := make([]byte, 32)
	f := bytes.Repeat([]byte{0xFF}, 32)

	for _, b := range values {
		ikm := append(f, b...)
		ikm = append(ikm, okm...)
		kdf := hkdf.New(sha256.New, ikm, salt, info)
		n, err := kdf.Read(okm)
		if err != nil {
			return nil, err
		}
		if n != len(okm) {
			return nil, fmt.Errorf("unable to read expected number of bytes want=%v got=%v", len(okm), n)
		}
		ByteSub(f)
	}
	return okm, nil
}

func anyNil(values ...[]byte) bool {
	for _, x := range values {
		if x == nil {
			return true
		}
	}
	return false
}

// ByteSub is a constant time algorithm for subtracting
// 1 from the array as if it were a big number.
// 0 is considered a wrap which resets to 0xFF
func ByteSub(b []byte) {
	m := byte(1)
	for i := 0; i < len(b); i++ {
		b[i] -= m

		// If b[i] > 0, s == 0
		// If b[i] == 0, s == 1
		// Computing IsNonZero(b[i])
		s1 := int8(b[i]) >> 7
		s2 := -int8(b[i]) >> 7
		s := byte((s1 | s2) + 1)

		// If s == 0, don't subtract anymore
		// s == 1, continue subtracting
		m = s & m
		// If s == 0 this does nothing
		// If s == 1 reset this value to 0xFF
		b[i] |= -s
	}
}
