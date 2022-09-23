//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bls_sig

import (
	"fmt"

	"github.com/coinbase/kryptology/internal"
	"github.com/coinbase/kryptology/pkg/core/curves/native"
	"github.com/coinbase/kryptology/pkg/core/curves/native/bls12381"
)

// Implement BLS signatures on the BLS12-381 curve
// according to https://crypto.standford.edu/~dabo/pubs/papers/BLSmultisig.html
// and https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
// this file implements signatures in G1 and public keys in G2.
// Public Keys and Signatures can be aggregated but the consumer
// must use proofs of possession to defend against rogue-key attacks.

const (
	// Public key size in G2
	PublicKeyVtSize = 96
	// Signature size in G1
	SignatureVtSize = 48
	// Proof of Possession in G1
	ProofOfPossessionVtSize = 48
)

// Represents a public key in G2
type PublicKeyVt struct {
	value bls12381.G2
}

// Serialize a public key to a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
func (pk *PublicKeyVt) MarshalBinary() ([]byte, error) {
	out := pk.value.ToCompressed()
	return out[:], nil
}

// Deserialize a public key from a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
// If successful, it will assign the public key
// otherwise it will return an error
func (pk *PublicKeyVt) UnmarshalBinary(data []byte) error {
	if len(data) != PublicKeyVtSize {
		return fmt.Errorf("public key must be %d bytes", PublicKeySize)
	}
	var blob [PublicKeyVtSize]byte
	copy(blob[:], data)
	p2, err := new(bls12381.G2).FromCompressed(&blob)
	if err != nil {
		return err
	}
	if p2.IsIdentity() == 1 {
		return fmt.Errorf("public keys cannot be zero")
	}
	pk.value = *p2
	return nil
}

// Represents a BLS signature in G1
type SignatureVt struct {
	value bls12381.G1
}

// Serialize a signature to a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
func (sig *SignatureVt) MarshalBinary() ([]byte, error) {
	out := sig.value.ToCompressed()
	return out[:], nil
}

func (sig *SignatureVt) verify(pk *PublicKeyVt, message []byte, signDstVt string) (bool, error) {
	return pk.verifySignatureVt(message, sig, signDstVt)
}

// The AggregateVerify algorithm checks an aggregated signature over
// several (PK, message) pairs.
// The Signature is the output of aggregateSignaturesVt
// Each message must be different or this will return false.
// See section 3.1.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (sig *SignatureVt) aggregateVerify(pks []*PublicKeyVt, msgs [][]byte, signDstVt string) (bool, error) {
	return sig.coreAggregateVerify(pks, msgs, signDstVt)
}

func (sig *SignatureVt) coreAggregateVerify(pks []*PublicKeyVt, msgs [][]byte, signDstVt string) (bool, error) {
	if len(pks) < 1 {
		return false, fmt.Errorf("at least one key is required")
	}
	if len(msgs) < 1 {
		return false, fmt.Errorf("at least one message is required")
	}
	if len(pks) != len(msgs) {
		return false, fmt.Errorf("the number of public keys does not match the number of messages: %v != %v", len(pks), len(msgs))
	}
	if sig.value.InCorrectSubgroup() == 0 {
		return false, fmt.Errorf("signature is not in the correct subgroup")
	}

	engine := new(bls12381.Engine)
	dst := []byte(signDstVt)
	// e(H(m_1), pk_1)*...*e(H(m_N), pk_N) == e(s, g2)
	// However, we use only one miller loop
	// by doing the equivalent of
	// e(H(m_1), pk_1)*...*e(H(m_N), pk_N) * e(s^-1, g2) == 1
	for i, pk := range pks {
		if pk == nil {
			return false, fmt.Errorf("public key at %d is nil", i)
		}
		if pk.value.IsIdentity() == 1 || pk.value.InCorrectSubgroup() == 0 {
			return false, fmt.Errorf("public key at %d is not in the correct subgroup", i)
		}
		p1 := new(bls12381.G1).Hash(native.EllipticPointHasherSha256(), msgs[i], dst)
		engine.AddPair(p1, &pk.value)
	}
	engine.AddPairInvG2(&sig.value, new(bls12381.G2).Generator())
	return engine.Check(), nil
}

// Deserialize a signature from a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
// If successful, it will assign the Signature
// otherwise it will return an error
func (sig *SignatureVt) UnmarshalBinary(data []byte) error {
	if len(data) != SignatureVtSize {
		return fmt.Errorf("signature must be %d bytes", SignatureSize)
	}
	var blob [SignatureVtSize]byte
	copy(blob[:], data)
	p1, err := new(bls12381.G1).FromCompressed(&blob)
	if err != nil {
		return err
	}
	if p1.IsIdentity() == 1 {
		return fmt.Errorf("signatures cannot be zero")
	}
	sig.value = *p1
	return nil
}

// Get the corresponding public key from a secret key
// Verifies the public key is in the correct subgroup
func (sk *SecretKey) GetPublicKeyVt() (*PublicKeyVt, error) {
	result := new(bls12381.G2).Mul(new(bls12381.G2).Generator(), sk.value)
	if result.InCorrectSubgroup() == 0 || result.IsIdentity() == 1 {
		return nil, fmt.Errorf("point is not in correct subgroup")
	}
	return &PublicKeyVt{value: *result}, nil
}

// Compute a signature from a secret key and message
// This signature is deterministic which protects against
// attacks arising from signing with bad randomness like
// the nonce reuse attack on ECDSA. `message` is
// hashed to a point in G1 as described in to
// https://datatracker.ietf.org/doc/draft-irtf-cfrg-hash-to-curve/?include_text=1
// See Section 2.6 in https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
// nil message is not permitted but empty slice is allowed
func (sk *SecretKey) createSignatureVt(message []byte, dstVt string) (*SignatureVt, error) {
	if message == nil {
		return nil, fmt.Errorf("message cannot be nil")
	}
	if sk.value.IsZero() == 1 {
		return nil, fmt.Errorf("invalid secret key")
	}
	p1 := new(bls12381.G1).Hash(native.EllipticPointHasherSha256(), message, []byte(dstVt))
	result := new(bls12381.G1).Mul(p1, sk.value)
	if result.InCorrectSubgroup() == 0 {
		return nil, fmt.Errorf("point is not in correct subgroup")
	}
	return &SignatureVt{value: *result}, nil
}

// Verify a signature is valid for the message under this public key.
// See Section 2.7 in https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (pk PublicKeyVt) verifySignatureVt(message []byte, signature *SignatureVt, dstVt string) (bool, error) {
	if signature == nil || message == nil || pk.value.IsIdentity() == 1 {
		return false, fmt.Errorf("signature and message and public key cannot be nil or zero")
	}
	if signature.value.IsIdentity() == 1 || signature.value.InCorrectSubgroup() == 0 {
		return false, fmt.Errorf("signature is not in the correct subgroup")
	}
	engine := new(bls12381.Engine)

	p1 := new(bls12381.G1).Hash(native.EllipticPointHasherSha256(), message, []byte(dstVt))
	// e(H(m), pk) == e(s, g2)
	// However, we can reduce the number of miller loops
	// by doing the equivalent of
	// e(H(m)^-1, pk) * e(s, g2) == 1
	engine.AddPairInvG1(p1, &pk.value)
	engine.AddPair(&signature.value, new(bls12381.G2).Generator())
	return engine.Check(), nil
}

// Combine public keys into one aggregated key
func aggregatePublicKeysVt(pks ...*PublicKeyVt) (*PublicKeyVt, error) {
	if len(pks) < 1 {
		return nil, fmt.Errorf("at least one public key is required")
	}
	result := new(bls12381.G2).Identity()
	for i, k := range pks {
		if k == nil {
			return nil, fmt.Errorf("key at %d is nil, keys cannot be nil", i)
		}
		if k.value.InCorrectSubgroup() == 0 {
			return nil, fmt.Errorf("key at %d is not in the correct subgroup", i)
		}
		result.Add(result, &k.value)
	}
	return &PublicKeyVt{value: *result}, nil
}

// Combine signatures into one aggregated signature
func aggregateSignaturesVt(sigs ...*SignatureVt) (*SignatureVt, error) {
	if len(sigs) < 1 {
		return nil, fmt.Errorf("at least one signature is required")
	}
	result := new(bls12381.G1).Identity()
	for i, s := range sigs {
		if s == nil {
			return nil, fmt.Errorf("signature at %d is nil, signature cannot be nil", i)
		}
		if s.value.InCorrectSubgroup() == 0 {
			return nil, fmt.Errorf("signature at %d is not in the correct subgroup", i)
		}
		result.Add(result, &s.value)
	}
	return &SignatureVt{value: *result}, nil
}

// A proof of possession scheme uses a separate public key validation
// step, called a proof of possession, to defend against rogue key
// attacks. This enables an optimization to aggregate signature
// verification for the case that all signatures are on the same
// message.
type ProofOfPossessionVt struct {
	value bls12381.G1
}

// Generates a proof-of-possession (PoP) for this secret key. The PoP signature should be verified before
// before accepting any aggregate signatures related to the corresponding pubkey.
func (sk *SecretKey) createProofOfPossessionVt(popDstVt string) (*ProofOfPossessionVt, error) {
	pk, err := sk.GetPublicKeyVt()
	if err != nil {
		return nil, err
	}
	msg, err := pk.MarshalBinary()
	if err != nil {
		return nil, err
	}
	sig, err := sk.createSignatureVt(msg, popDstVt)
	if err != nil {
		return nil, err
	}
	return &ProofOfPossessionVt{value: sig.value}, nil
}

// Serialize a proof of possession to a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
func (pop *ProofOfPossessionVt) MarshalBinary() ([]byte, error) {
	out := pop.value.ToCompressed()
	return out[:], nil
}

// Deserialize a proof of possession from a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
// If successful, it will assign the Signature
// otherwise it will return an error
func (pop *ProofOfPossessionVt) UnmarshalBinary(data []byte) error {
	p1 := new(SignatureVt)
	err := p1.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	pop.value = p1.value
	return nil
}

// Verifies that PoP is valid for this pubkey. In order to prevent rogue key attacks, a PoP must be validated
// for each pubkey in an aggregated signature.
func (pop *ProofOfPossessionVt) verify(pk *PublicKeyVt, popDstVt string) (bool, error) {
	if pk == nil {
		return false, fmt.Errorf("public key cannot be nil")
	}
	msg, err := pk.MarshalBinary()
	if err != nil {
		return false, err
	}
	return pk.verifySignatureVt(msg, &SignatureVt{value: pop.value}, popDstVt)
}

// Represents an MultiSignature in G1. A multisignature is used when multiple signatures
// are calculated over the same message vs an aggregate signature where each message signed
// is a unique.
type MultiSignatureVt struct {
	value bls12381.G1
}

// Serialize a multi-signature to a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
func (sig *MultiSignatureVt) MarshalBinary() ([]byte, error) {
	out := sig.value.ToCompressed()
	return out[:], nil
}

// Check a multisignature is valid for a multipublickey and a message
func (sig *MultiSignatureVt) verify(pk *MultiPublicKeyVt, message []byte, signDstVt string) (bool, error) {
	if pk == nil {
		return false, fmt.Errorf("public key cannot be nil")
	}
	p := &PublicKeyVt{value: pk.value}
	return p.verifySignatureVt(message, &SignatureVt{value: sig.value}, signDstVt)
}

// Deserialize a signature from a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
// If successful, it will assign the Signature
// otherwise it will return an error
func (sig *MultiSignatureVt) UnmarshalBinary(data []byte) error {
	if len(data) != SignatureVtSize {
		return fmt.Errorf("multi signature must be %v bytes", SignatureSize)
	}
	s1 := new(SignatureVt)
	err := s1.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	sig.value = s1.value
	return nil
}

// Represents accumulated multiple Public Keys in G2 for verifying a multisignature
type MultiPublicKeyVt struct {
	value bls12381.G2
}

// Serialize a public key to a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
func (pk *MultiPublicKeyVt) MarshalBinary() ([]byte, error) {
	out := pk.value.ToCompressed()
	return out[:], nil
}

// Deserialize a public key from a byte array in compressed form.
// See
// https://github.com/zcash/librustzcash/blob/master/pairing/src/bls12_381/README.md#serialization
// https://docs.rs/bls12_381/0.1.1/bls12_381/notes/serialization/index.html
// If successful, it will assign the public key
// otherwise it will return an error
func (pk *MultiPublicKeyVt) UnmarshalBinary(data []byte) error {
	if len(data) != PublicKeyVtSize {
		return fmt.Errorf("multi public key must be %v bytes", PublicKeySize)
	}
	p2 := new(PublicKeyVt)
	err := p2.UnmarshalBinary(data)
	if err != nil {
		return err
	}
	pk.value = p2.value
	return nil
}

// Check a multisignature is valid for a multipublickey and a message
func (pk *MultiPublicKeyVt) verify(message []byte, sig *MultiSignatureVt, signDstVt string) (bool, error) {
	return sig.verify(pk, message, signDstVt)
}

// PartialSignatureVt represents threshold Gap Diffie-Hellman BLS signature
// that can be combined with other partials to yield a completed BLS signature
// See section 3.2 in <https://www.cc.gatech.edu/~aboldyre/papers/bold.pdf>
type PartialSignatureVt struct {
	identifier byte
	signature  bls12381.G1
}

// partialSignVt creates a partial signature that can be combined with other partial signatures
// to yield a complete signature
func (sks *SecretKeyShare) partialSignVt(message []byte, signDst string) (*PartialSignatureVt, error) {
	if len(message) == 0 {
		return nil, fmt.Errorf("message cannot be empty or nil")
	}
	p1 := new(bls12381.G1).Hash(native.EllipticPointHasherSha256(), message, []byte(signDst))

	var blob [SecretKeySize]byte
	copy(blob[:], internal.ReverseScalarBytes(sks.value))
	s, err := bls12381.Bls12381FqNew().SetBytes(&blob)
	if err != nil {
		return nil, err
	}
	result := new(bls12381.G1).Mul(p1, s)
	if result.InCorrectSubgroup() == 0 {
		return nil, fmt.Errorf("point is not on correct subgroup")
	}
	return &PartialSignatureVt{identifier: sks.identifier, signature: *result}, nil
}

// combineSigsVt gathers partial signatures and yields a complete signature
func combineSigsVt(partials []*PartialSignatureVt) (*SignatureVt, error) {
	if len(partials) < 2 {
		return nil, fmt.Errorf("must have at least 2 partial signatures")
	}
	if len(partials) > 255 {
		return nil, fmt.Errorf("unsupported to combine more than 255 signatures")
	}

	xVars, yVars, err := splitXYVt(partials)

	if err != nil {
		return nil, err
	}

	sTmp := new(bls12381.G1).Identity()
	sig := new(bls12381.G1).Identity()

	// Lagrange interpolation
	basis := bls12381.Bls12381FqNew()
	for i, xi := range xVars {
		basis.SetOne()

		for j, xj := range xVars {
			if i == j {
				continue
			}

			num := bls12381.Bls12381FqNew().Neg(xj)     // x - x_m
			den := bls12381.Bls12381FqNew().Sub(xi, xj) // x_j - x_m
			_, wasInverted := den.Invert(den)
			// wasInverted == false if den == 0
			if !wasInverted {
				return nil, fmt.Errorf("signatures cannot be recombined")
			}
			basis.Mul(basis, num.Mul(num, den))
		}
		sTmp.Mul(yVars[i], basis)
		sig.Add(sig, sTmp)
	}
	if sig.InCorrectSubgroup() == 0 {
		return nil, fmt.Errorf("signature is not in the correct subgroup")
	}

	return &SignatureVt{value: *sig}, nil
}

// Ensure no duplicates x values and convert x values to field elements
func splitXYVt(partials []*PartialSignatureVt) ([]*native.Field, []*bls12381.G1, error) {
	x := make([]*native.Field, len(partials))
	y := make([]*bls12381.G1, len(partials))

	dup := make(map[byte]bool)

	for i, sp := range partials {
		if sp == nil {
			return nil, nil, fmt.Errorf("partial signature cannot be nil")
		}
		if _, exists := dup[sp.identifier]; exists {
			return nil, nil, fmt.Errorf("duplicate signature included")
		}
		if sp.signature.InCorrectSubgroup() == 0 {
			return nil, nil, fmt.Errorf("signature is not in the correct subgroup")
		}
		dup[sp.identifier] = true
		x[i] = bls12381.Bls12381FqNew().SetUint64(uint64(sp.identifier))
		y[i] = &sp.signature
	}
	return x, y, nil
}
