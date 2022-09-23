//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package core

import (
	"bytes"
	"crypto/elliptic"
	"crypto/sha256"
	"fmt"
	"hash"
	"math"
	"math/big"

	"github.com/btcsuite/btcd/btcec"
	"golang.org/x/crypto/hkdf"

	"github.com/coinbase/kryptology/internal"
)

type HashField struct {
	// F_p^k
	Order           *big.Int // p^k
	Characteristic  *big.Int // p
	ExtensionDegree *big.Int // k
}

type Params struct {
	F                 *HashField
	SecurityParameter int
	Hash              func() hash.Hash
	L                 int
}

func getParams(curve elliptic.Curve) (*Params, error) {
	switch curve.Params().Name {
	case btcec.S256().Name, elliptic.P256().Params().Name:
		return &Params{
			F: &HashField{
				Order:           curve.Params().P,
				Characteristic:  curve.Params().P,
				ExtensionDegree: new(big.Int).SetInt64(1),
			},
			SecurityParameter: 128,
			Hash:              sha256.New,
			L:                 48,
		}, nil
	case "Bls12381G1":
		return &Params{
			F: &HashField{
				Order:           curve.Params().P,
				Characteristic:  curve.Params().P,
				ExtensionDegree: new(big.Int).SetInt64(1),
			},
			SecurityParameter: 128,
			Hash:              sha256.New,
			L:                 48,
		}, nil
	case "ed25519":
		return &Params{
			F: &HashField{
				Order:           curve.Params().P,
				Characteristic:  curve.Params().P,
				ExtensionDegree: new(big.Int).SetInt64(1),
			},
			SecurityParameter: 128,
			Hash:              sha256.New,
			L:                 48,
		}, nil
	default:
		return nil, fmt.Errorf("Not implemented: %s", curve.Params().Name)
	}
}

func I2OSP(b, n int) []byte {
	os := new(big.Int).SetInt64(int64(b)).Bytes()
	if n > len(os) {
		var buf bytes.Buffer
		buf.Write(make([]byte, n-len(os)))
		buf.Write(os)
		return buf.Bytes()
	}
	return os[:n]
}

func OS2IP(os []byte) *big.Int {
	return new(big.Int).SetBytes(os)
}

func hashThis(f func() hash.Hash, this []byte) ([]byte, error) {
	h := f()
	w, err := h.Write(this)
	if w != len(this) {
		return nil, fmt.Errorf("bytes written to hash doesn't match expected")
	} else if err != nil {
		return nil, err
	}
	v := h.Sum(nil)
	return v, nil
}

func concat(xs ...[]byte) []byte {
	var result []byte
	for _, x := range xs {
		result = append(result, x...)
	}
	return result
}

func xor(b1, b2 []byte) []byte {
	// b1 and b2 must be same length
	result := make([]byte, len(b1))
	for i := range b1 {
		result[i] = b1[i] ^ b2[i]
	}

	return result
}

func ExpandMessageXmd(f func() hash.Hash, msg, DST []byte, lenInBytes int) ([]byte, error) {
	// https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-10#section-5.4.1

	// step 1
	ell := int(math.Ceil(float64(lenInBytes) / float64(f().Size())))

	//step 2
	if ell > 255 {
		return nil, fmt.Errorf("ell > 255")
	}

	// step 3
	dstPrime := append(DST, I2OSP(len(DST), 1)...)

	// step 4
	zPad := I2OSP(0, f().BlockSize())

	// step 5 & 6
	msgPrime := concat(zPad, msg, I2OSP(lenInBytes, 2), I2OSP(0, 1), dstPrime)

	var err error

	b := make([][]byte, ell+1)

	// step 7
	b[0], err = hashThis(f, msgPrime)
	if err != nil {
		return nil, err
	}

	// step 8
	b[1], err = hashThis(f, concat(b[0], I2OSP(1, 1), dstPrime))
	if err != nil {
		return nil, err
	}

	// step 9
	for i := 2; i <= ell; i++ {
		// step 10
		b[i], err = hashThis(f, concat(xor(b[0], b[i-1]), I2OSP(i, 1), dstPrime))
		if err != nil {
			return nil, err
		}
	}
	// step 11
	uniformBytes := concat(b[1:]...)

	// step 12
	return uniformBytes[:lenInBytes], nil
}

func hashToField(msg []byte, count int, curve elliptic.Curve) ([][]*big.Int, error) {
	// https://tools.ietf.org/html/draft-irtf-cfrg-hash-to-curve-10#section-5.3

	parameters, err := getParams(curve)
	if err != nil {
		return nil, err
	}

	f := parameters.Hash

	DST := []byte("Coinbase_tECDSA")

	m := int(parameters.F.ExtensionDegree.Int64())
	L := parameters.L

	// step 1
	lenInBytes := count * m * L

	// step 2
	uniformBytes, err := ExpandMessageXmd(f, msg, DST, lenInBytes)
	if err != nil {
		return nil, err
	}

	u := make([][]*big.Int, count)

	// step 3
	for i := 0; i < count; i++ {
		e := make([]*big.Int, m)
		// step 4
		for j := 0; j < m; j++ {
			// step 5
			elmOffset := L * (j + i*m)
			// step 6
			tv := uniformBytes[elmOffset : elmOffset+L]
			// step 7
			e[j] = new(big.Int).Mod(OS2IP(tv), parameters.F.Characteristic)

		}
		// step 8
		u[i] = e
	}
	// step 9
	return u, nil
}

func Hash(msg []byte, curve elliptic.Curve) (*big.Int, error) {
	u, err := hashToField(msg, 1, curve)
	if err != nil {
		return nil, err
	}
	return u[0][0], nil
}

// fiatShamir computes the HKDF over many values
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
func FiatShamir(values ...*big.Int) ([]byte, error) {
	// Don't accept any nil arguments
	if AnyNil(values...) {
		return nil, internal.ErrNilArguments
	}

	info := []byte("Coinbase tECDSA 1.0")
	salt := make([]byte, 32)
	okm := make([]byte, 32)
	f := bytes.Repeat([]byte{0xFF}, 32)

	for _, b := range values {
		ikm := append(f, b.Bytes()...)
		ikm = append(ikm, okm...)
		kdf := hkdf.New(sha256.New, ikm, salt, info)
		n, err := kdf.Read(okm)
		if err != nil {
			return nil, err
		}
		if n != len(okm) {
			return nil, fmt.Errorf("unable to read expected number of bytes want=%v got=%v", len(okm), n)
		}
		internal.ByteSub(f)
	}
	return okm, nil
}
