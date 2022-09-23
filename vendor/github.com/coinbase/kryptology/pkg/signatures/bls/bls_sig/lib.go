//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

// Package bls_sig is an implementation of the BLS signature defined in https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
package bls_sig

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"

	"golang.org/x/crypto/hkdf"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	"github.com/coinbase/kryptology/pkg/core/curves/native/bls12381"
	"github.com/coinbase/kryptology/pkg/sharing"
)

// Secret key in Fr
const SecretKeySize = 32

// Secret key share with identifier byte in Fr
const SecretKeyShareSize = 33

// The salt used with generating secret keys
// See section 2.3 from https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
const hkdfKeyGenSalt = "BLS-SIG-KEYGEN-SALT-"

// Represents a value mod r where r is the curve order or
// order of the subgroups in G1 and G2
type SecretKey struct {
	value *native.Field
}

func allRowsUnique(data [][]byte) bool {
	seen := make(map[string]bool)
	for _, row := range data {
		m := string(row)
		if _, ok := seen[m]; ok {
			return false
		}
		seen[m] = true
	}
	return true
}

func generateRandBytes(count int) ([]byte, error) {
	ikm := make([]byte, count)
	cnt, err := rand.Read(ikm)
	if err != nil {
		return nil, err
	}
	if cnt != count {
		return nil, fmt.Errorf("unable to read sufficient random data")
	}
	return ikm, nil
}

// Creates a new BLS secret key
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (sk SecretKey) Generate(ikm []byte) (*SecretKey, error) {
	if len(ikm) < 32 {
		return nil, fmt.Errorf("ikm is too short. Must be at least 32")
	}

	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-04#section-2.3
	h := sha256.New()
	n, err := h.Write([]byte(hkdfKeyGenSalt))
	if err != nil {
		return nil, err
	}
	if n != len(hkdfKeyGenSalt) {
		return nil, fmt.Errorf("incorrect salt bytes written to be hashed")
	}
	salt := h.Sum(nil)

	ikm = append(ikm, 0)
	// Leaves key_info parameter as the default empty string
	// and just adds parameter I2OSP(L, 2)
	var okm [native.WideFieldBytes]byte
	kdf := hkdf.New(sha256.New, ikm, salt, []byte{0, 48})
	read, err := kdf.Read(okm[:48])
	copy(okm[:48], internal.ReverseScalarBytes(okm[:48]))
	if err != nil {
		return nil, err
	}
	if read != 48 {
		return nil, fmt.Errorf("failed to create private key")
	}
	v := bls12381.Bls12381FqNew().SetBytesWide(&okm)
	return &SecretKey{value: v}, nil
}

// Serialize a secret key to raw bytes
func (sk SecretKey) MarshalBinary() ([]byte, error) {
	bytes := sk.value.Bytes()
	return internal.ReverseScalarBytes(bytes[:]), nil
}

// Deserialize a secret key from raw bytes
// Cannot be zero. Must be 32 bytes and cannot be all zeroes.
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-2.3
func (sk *SecretKey) UnmarshalBinary(data []byte) error {
	if len(data) != SecretKeySize {
		return fmt.Errorf("secret key must be %d bytes", SecretKeySize)
	}
	zeros := make([]byte, len(data))
	if subtle.ConstantTimeCompare(data, zeros) == 1 {
		return fmt.Errorf("secret key cannot be zero")
	}
	var bb [native.FieldBytes]byte
	copy(bb[:], internal.ReverseScalarBytes(data))
	value, err := bls12381.Bls12381FqNew().SetBytes(&bb)
	if err != nil {
		return err
	}
	sk.value = value
	return nil
}

// SecretKeyShare is shamir share of a private key
type SecretKeyShare struct {
	identifier byte
	value      []byte
}

// Serialize a secret key share to raw bytes
func (sks SecretKeyShare) MarshalBinary() ([]byte, error) {
	var blob [SecretKeyShareSize]byte
	l := len(sks.value)
	copy(blob[:l], sks.value)
	blob[l] = sks.identifier
	return blob[:], nil
}

// Deserialize a secret key share from raw bytes
func (sks *SecretKeyShare) UnmarshalBinary(data []byte) error {
	if len(data) != SecretKeyShareSize {
		return fmt.Errorf("secret key share must be %d bytes", SecretKeyShareSize)
	}

	zeros := make([]byte, len(data))
	if subtle.ConstantTimeCompare(data, zeros) == 1 {
		return fmt.Errorf("secret key share cannot be zero")
	}
	l := len(data)
	sks.identifier = data[l-1]
	copy(sks.value, data[:l])
	return nil
}

// thresholdizeSecretKey splits a composite secret key such that
// `threshold` partial signatures can be combined to form a composite signature
func thresholdizeSecretKey(secretKey *SecretKey, threshold, total uint) ([]*SecretKeyShare, error) {
	// Verify our parameters are acceptable.
	if secretKey == nil {
		return nil, fmt.Errorf("secret key is nil")
	}
	if threshold > total {
		return nil, fmt.Errorf("threshold cannot be greater than the total")
	}
	if threshold == 0 {
		return nil, fmt.Errorf("threshold cannot be zero")
	}
	if total <= 1 {
		return nil, fmt.Errorf("total must be larger than 1")
	}
	if total > 255 || threshold > 255 {
		return nil, fmt.Errorf("cannot have more than 255 shares")
	}

	curve := curves.BLS12381G1()
	sss, err := sharing.NewShamir(uint32(threshold), uint32(total), curve)
	if err != nil {
		return nil, err
	}
	secret, ok := curve.NewScalar().(*curves.ScalarBls12381)
	if !ok {
		return nil, fmt.Errorf("invalid curve")
	}
	secret.Value = secretKey.value
	shares, err := sss.Split(secret, rand.Reader)
	if err != nil {
		return nil, err
	}
	// Verify we got the expected number of shares
	if uint(len(shares)) != total {
		return nil, fmt.Errorf("%v != %v shares", len(shares), total)
	}

	// Package our shares
	secrets := make([]*SecretKeyShare, len(shares))
	for i, s := range shares {
		// users expect BigEndian
		sks := &SecretKeyShare{identifier: byte(s.Id), value: s.Value}
		secrets[i] = sks
	}

	return secrets, nil
}
