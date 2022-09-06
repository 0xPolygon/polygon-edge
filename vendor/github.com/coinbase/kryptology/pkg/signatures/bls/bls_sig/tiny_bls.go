//
// Copyright Coinbase, Inc. All Rights Reserved.
//
// SPDX-License-Identifier: Apache-2.0
//

package bls_sig

import (
	"fmt"
)

const (
	// Domain separation tag for basic signatures
	// according to section 4.2.1 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsSignatureBasicVtDst = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_NUL_"
	// Domain separation tag for basic signatures
	// according to section 4.2.2 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsSignatureAugVtDst = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_AUG_"
	// Domain separation tag for proof of possession signatures
	// according to section 4.2.3 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsSignaturePopVtDst = "BLS_SIG_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
	// Domain separation tag for proof of possession proofs
	// according to section 4.2.3 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsPopProofVtDst = "BLS_POP_BLS12381G1_XMD:SHA-256_SSWU_RO_POP_"
)

type BlsSchemeVt interface {
	Keygen() (*PublicKeyVt, *SecretKey, error)
	KeygenWithSeed(ikm []byte) (*PublicKeyVt, *SecretKey, error)
	Sign(sk *SecretKey, msg []byte) (*SignatureVt, error)
	Verify(pk *PublicKeyVt, msg []byte, sig *SignatureVt) bool
	AggregateVerify(pks []*PublicKeyVt, msgs [][]byte, sigs []*SignatureVt) bool
}

// generateKeysVt creates 32 bytes of random data to be fed to
// generateKeysWithSeedVt
func generateKeysVt() (*PublicKeyVt, *SecretKey, error) {
	ikm, err := generateRandBytes(32)
	if err != nil {
		return nil, nil, err
	}
	return generateKeysWithSeedVt(ikm)
}

// generateKeysWithSeedVt generates a BLS key pair given input key material (ikm)
func generateKeysWithSeedVt(ikm []byte) (*PublicKeyVt, *SecretKey, error) {
	sk, err := new(SecretKey).Generate(ikm)
	if err != nil {
		return nil, nil, err
	}
	pk, err := sk.GetPublicKeyVt()
	if err != nil {
		return nil, nil, err
	}
	return pk, sk, nil
}

// thresholdGenerateKeys will generate random secret key shares and the corresponding public key
func thresholdGenerateKeysVt(threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	pk, sk, err := generateKeysVt()
	if err != nil {
		return nil, nil, err
	}
	shares, err := thresholdizeSecretKey(sk, threshold, total)
	if err != nil {
		return nil, nil, err
	}
	return pk, shares, nil
}

// thresholdGenerateKeysWithSeed will generate random secret key shares and the corresponding public key
// using the corresponding seed `ikm`
func thresholdGenerateKeysWithSeedVt(ikm []byte, threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	pk, sk, err := generateKeysWithSeedVt(ikm)
	if err != nil {
		return nil, nil, err
	}
	shares, err := thresholdizeSecretKey(sk, threshold, total)
	if err != nil {
		return nil, nil, err
	}
	return pk, shares, nil
}

// SigBasic is minimal-pubkey-size scheme that doesn't support FastAggregateVerification.
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.1
type SigBasicVt struct {
	dst string
}

// Creates a new BLS basic signature scheme with the standard domain separation tag used for signatures.
func NewSigBasicVt() *SigBasicVt {
	return &SigBasicVt{dst: blsSignatureBasicVtDst}
}

// Creates a new BLS basic signature scheme with a custom domain separation tag used for signatures.
func NewSigBasicVtWithDst(signDst string) *SigBasicVt {
	return &SigBasicVt{dst: signDst}
}

// Creates a new BLS key pair
func (b SigBasicVt) Keygen() (*PublicKeyVt, *SecretKey, error) {
	return generateKeysVt()
}

// Creates a new BLS key pair
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigBasicVt) KeygenWithSeed(ikm []byte) (*PublicKeyVt, *SecretKey, error) {
	return generateKeysWithSeedVt(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigBasicVt) ThresholdKeygen(threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysVt(threshold, total)
}

// ThresholdKeygenWithSeed generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures from input key material (ikm)
func (b SigBasicVt) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeedVt(ikm, threshold, total)
}

// Computes a signature in G1 from sk, a secret key, and a message
func (b SigBasicVt) Sign(sk *SecretKey, msg []byte) (*SignatureVt, error) {
	return sk.createSignatureVt(msg, b.dst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigBasicVt) PartialSign(sks *SecretKeyShare, msg []byte) (*PartialSignatureVt, error) {
	return sks.partialSignVt(msg, b.dst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigBasicVt) CombineSignatures(sigs ...*PartialSignatureVt) (*SignatureVt, error) {
	return combineSigsVt(sigs)
}

// Checks that a signature is valid for the message under the public key pk
func (b SigBasicVt) Verify(pk *PublicKeyVt, msg []byte, sig *SignatureVt) (bool, error) {
	return pk.verifySignatureVt(msg, sig, b.dst)
}

// The AggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// Each message must be different or this will return false.
// See section 3.1.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigBasicVt) AggregateVerify(pks []*PublicKeyVt, msgs [][]byte, sigs []*SignatureVt) (bool, error) {
	if !allRowsUnique(msgs) {
		return false, fmt.Errorf("all messages must be distinct")
	}
	asig, err := aggregateSignaturesVt(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, msgs, b.dst)
}

// SigAugVt is minimal-signature-size scheme that doesn't support FastAggregateVerification.
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.2
type SigAugVt struct {
	dst string
}

// Creates a new BLS message augmentation signature scheme with the standard domain separation tag used for signatures.
func NewSigAugVt() *SigAugVt {
	return &SigAugVt{dst: blsSignatureAugVtDst}
}

// Creates a new BLS message augmentation signature scheme with a custom domain separation tag used for signatures.
func NewSigAugVtWithDst(signDst string) *SigAugVt {
	return &SigAugVt{dst: signDst}
}

// Creates a new BLS key pair
func (b SigAugVt) Keygen() (*PublicKeyVt, *SecretKey, error) {
	return generateKeysVt()
}

// Creates a new BLS secret key
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigAugVt) KeygenWithSeed(ikm []byte) (*PublicKeyVt, *SecretKey, error) {
	return generateKeysWithSeedVt(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigAugVt) ThresholdKeygen(threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysVt(threshold, total)
}

// ThresholdKeygenWithSeed generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigAugVt) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeedVt(ikm, threshold, total)
}

// Computes a signature in G1 from sk, a secret key, and a message
// See section 3.2.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-02
func (b SigAugVt) Sign(sk *SecretKey, msg []byte) (*SignatureVt, error) {
	pk, err := sk.GetPublicKeyVt()
	if err != nil {
		return nil, err
	}
	bytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("MarshalBinary failed")
	}
	bytes = append(bytes, msg...)
	return sk.createSignatureVt(bytes, b.dst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigAugVt) PartialSign(sks *SecretKeyShare, pk *PublicKeyVt, msg []byte) (*PartialSignatureVt, error) {
	if len(msg) == 0 {
		return nil, fmt.Errorf("message cannot be empty or nil")
	}
	bytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("MarshalBinary failed")
	}
	bytes = append(bytes, msg...)
	return sks.partialSignVt(bytes, b.dst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigAugVt) CombineSignatures(sigs ...*PartialSignatureVt) (*SignatureVt, error) {
	return combineSigsVt(sigs)
}

// Checks that a signature is valid for the message under the public key pk
// See section 3.2.2 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigAugVt) Verify(pk *PublicKeyVt, msg []byte, sig *SignatureVt) (bool, error) {
	bytes, err := pk.MarshalBinary()
	if err != nil {
		return false, err
	}
	bytes = append(bytes, msg...)
	return pk.verifySignatureVt(bytes, sig, b.dst)
}

// The aggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// See section 3.2.3 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigAugVt) AggregateVerify(pks []*PublicKeyVt, msgs [][]byte, sigs []*SignatureVt) (bool, error) {
	if len(pks) != len(msgs) {
		return false, fmt.Errorf("the number of public keys does not match the number of messages: %v != %v", len(pks), len(msgs))
	}
	data := make([][]byte, len(msgs))
	for i, msg := range msgs {
		bytes, err := pks[i].MarshalBinary()
		if err != nil {
			return false, err
		}
		data[i] = append(bytes, msg...)
	}
	asig, err := aggregateSignaturesVt(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, data, b.dst)
}

// SigEth2Vt supports signatures on Eth2.
// Internally is an alias for SigPopVt
type SigEth2Vt = SigPopVt

// NewSigEth2Vt Creates a new BLS ETH2 signature scheme with the standard domain separation tag used for signatures.
func NewSigEth2Vt() *SigEth2Vt {
	return NewSigPopVt()
}

// SigPopVt is minimal-signature-size scheme that supports FastAggregateVerification
// and requires using proofs of possession to mitigate rogue-key attacks
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.3
type SigPopVt struct {
	sigDst string
	popDst string
}

// Creates a new BLS proof of possession signature scheme with the standard domain separation tag used for signatures.
func NewSigPopVt() *SigPopVt {
	return &SigPopVt{sigDst: blsSignaturePopVtDst, popDst: blsPopProofVtDst}
}

// Creates a new BLS message proof of possession signature scheme with a custom domain separation tag used for signatures.
func NewSigPopVtWithDst(signDst, popDst string) (*SigPopVt, error) {
	if signDst == popDst {
		return nil, fmt.Errorf("domain separation tags cannot be equal")
	}
	return &SigPopVt{sigDst: signDst, popDst: popDst}, nil
}

// Creates a new BLS key pair
func (b SigPopVt) Keygen() (*PublicKeyVt, *SecretKey, error) {
	return generateKeysVt()
}

// Creates a new BLS secret key
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigPopVt) KeygenWithSeed(ikm []byte) (*PublicKeyVt, *SecretKey, error) {
	return generateKeysWithSeedVt(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigPopVt) ThresholdKeygen(threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysVt(threshold, total)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigPopVt) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKeyVt, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeedVt(ikm, threshold, total)
}

// Computes a signature in G1 from sk, a secret key, and a message
// See section 2.6 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) Sign(sk *SecretKey, msg []byte) (*SignatureVt, error) {
	return sk.createSignatureVt(msg, b.sigDst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigPopVt) PartialSign(sks *SecretKeyShare, msg []byte) (*PartialSignatureVt, error) {
	return sks.partialSignVt(msg, b.sigDst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigPopVt) CombineSignatures(sigs ...*PartialSignatureVt) (*SignatureVt, error) {
	return combineSigsVt(sigs)
}

// Checks that a signature is valid for the message under the public key pk
// See section 2.7 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) Verify(pk *PublicKeyVt, msg []byte, sig *SignatureVt) (bool, error) {
	return pk.verifySignatureVt(msg, sig, b.sigDst)
}

// The aggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// Each message must be different or this will return false.
// See section 3.1.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-02
func (b SigPopVt) AggregateVerify(pks []*PublicKeyVt, msgs [][]byte, sigs []*SignatureVt) (bool, error) {
	if !allRowsUnique(msgs) {
		return false, fmt.Errorf("all messages must be distinct")
	}
	asig, err := aggregateSignaturesVt(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, msgs, b.sigDst)
}

// Combine many signatures together to form a Multisignature.
// Multisignatures can be created when multiple signers jointly
// generate signatures over the same message.
func (b SigPopVt) AggregateSignatures(sigs ...*SignatureVt) (*MultiSignatureVt, error) {
	g1, err := aggregateSignaturesVt(sigs...)
	if err != nil {
		return nil, err
	}
	return &MultiSignatureVt{value: g1.value}, nil
}

// Combine many public keys together to form a Multipublickey.
// Multipublickeys are used to verify multisignatures.
func (b SigPopVt) AggregatePublicKeys(pks ...*PublicKeyVt) (*MultiPublicKeyVt, error) {
	g2, err := aggregatePublicKeysVt(pks...)
	if err != nil {
		return nil, err
	}
	return &MultiPublicKeyVt{value: g2.value}, nil
}

// Checks that a multisignature is valid for the message under the multi public key
// Similar to FastAggregateVerify except the keys and signatures have already been
// combined. See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-02
func (b SigPopVt) VerifyMultiSignature(pk *MultiPublicKeyVt, msg []byte, sig *MultiSignatureVt) (bool, error) {
	s := &SignatureVt{value: sig.value}
	p := &PublicKeyVt{value: pk.value}
	return p.verifySignatureVt(msg, s, b.sigDst)
}

// FastAggregateVerify verifies an aggregated signature over the same message under the given public keys.
// See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) FastAggregateVerify(pks []*PublicKeyVt, msg []byte, asig *SignatureVt) (bool, error) {
	apk, err := aggregatePublicKeysVt(pks...)
	if err != nil {
		return false, err
	}
	return apk.verifySignatureVt(msg, asig, b.sigDst)
}

// FastAggregateVerifyConstituent verifies a list of signature over the same message under the given public keys.
// See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) FastAggregateVerifyConstituent(pks []*PublicKeyVt, msg []byte, sigs []*SignatureVt) (bool, error) {
	asig, err := aggregateSignaturesVt(sigs...)
	if err != nil {
		return false, err
	}
	return b.FastAggregateVerify(pks, msg, asig)
}

// Create a proof of possession for the corresponding public key.
// A proof of possession must be created for each public key to be used
// in FastAggregateVerify or a Multipublickey to avoid rogue key attacks.
// See section 3.3.2 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) PopProve(sk *SecretKey) (*ProofOfPossessionVt, error) {
	return sk.createProofOfPossessionVt(b.popDst)
}

// verify a proof of possession for the corresponding public key is valid.
// A proof of possession must be created for each public key to be used
// in FastAggregateVerify or a Multipublickey to avoid rogue key attacks.
// See section 3.3.3 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPopVt) PopVerify(pk *PublicKeyVt, pop1 *ProofOfPossessionVt) (bool, error) {
	return pop1.verify(pk, b.popDst)
}
