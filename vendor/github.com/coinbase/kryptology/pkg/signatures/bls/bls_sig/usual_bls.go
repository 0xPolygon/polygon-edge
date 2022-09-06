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
	blsSignatureBasicDst = "BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"
	// Domain separation tag for basic signatures
	// according to section 4.2.2 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsSignatureAugDst = "BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_AUG_"
	// Domain separation tag for proof of possession signatures
	// according to section 4.2.3 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsSignaturePopDst = "BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"
	// Domain separation tag for proof of possession proofs
	// according to section 4.2.3 in
	// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
	blsPopProofDst = "BLS_POP_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"
)

type BlsScheme interface {
	Keygen() (*PublicKey, *SecretKey, error)
	KeygenWithSeed(ikm []byte) (*PublicKey, *SecretKey, error)
	Sign(sk *SecretKey, msg []byte) (*Signature, error)
	Verify(pk *PublicKey, msg []byte, sig *Signature) bool
	AggregateVerify(pks []*PublicKey, msgs [][]byte, sigs []*Signature) bool
}

// generateKeys creates 32 bytes of random data to be fed to
// generateKeysWithSeed
func generateKeys() (*PublicKey, *SecretKey, error) {
	ikm, err := generateRandBytes(32)
	if err != nil {
		return nil, nil, err
	}
	return generateKeysWithSeed(ikm)
}

// generateKeysWithSeed generates a BLS key pair given input key material (ikm)
func generateKeysWithSeed(ikm []byte) (*PublicKey, *SecretKey, error) {
	sk, err := new(SecretKey).Generate(ikm)
	if err != nil {
		return nil, nil, err
	}
	pk, err := sk.GetPublicKey()
	if err != nil {
		return nil, nil, err
	}
	return pk, sk, nil
}

// thresholdGenerateKeys will generate random secret key shares and the corresponding public key
func thresholdGenerateKeys(threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	pk, sk, err := generateKeys()
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
func thresholdGenerateKeysWithSeed(ikm []byte, threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	pk, sk, err := generateKeysWithSeed(ikm)
	if err != nil {
		return nil, nil, err
	}
	shares, err := thresholdizeSecretKey(sk, threshold, total)
	if err != nil {
		return nil, nil, err
	}
	return pk, shares, nil
}

// SigBasic is minimal-pubkey-size scheme that doesn't support FastAggregateVerificiation.
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.1
type SigBasic struct {
	dst string
}

// Creates a new BLS basic signature scheme with the standard domain separation tag used for signatures.
func NewSigBasic() *SigBasic {
	return &SigBasic{dst: blsSignatureBasicDst}
}

// Creates a new BLS basic signature scheme with a custom domain separation tag used for signatures.
func NewSigBasicWithDst(signDst string) *SigBasic {
	return &SigBasic{dst: signDst}
}

// Creates a new BLS key pair
func (b SigBasic) Keygen() (*PublicKey, *SecretKey, error) {
	return generateKeys()
}

// Creates a new BLS key pair
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigBasic) KeygenWithSeed(ikm []byte) (*PublicKey, *SecretKey, error) {
	return generateKeysWithSeed(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigBasic) ThresholdKeygen(threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeys(threshold, total)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigBasic) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeed(ikm, threshold, total)
}

// Computes a signature in G2 from sk, a secret key, and a message
func (b SigBasic) Sign(sk *SecretKey, msg []byte) (*Signature, error) {
	return sk.createSignature(msg, b.dst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigBasic) PartialSign(sks *SecretKeyShare, msg []byte) (*PartialSignature, error) {
	return sks.partialSign(msg, b.dst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigBasic) CombineSignatures(sigs ...*PartialSignature) (*Signature, error) {
	return combineSigs(sigs)
}

// Checks that a signature is valid for the message under the public key pk
func (b SigBasic) Verify(pk *PublicKey, msg []byte, sig *Signature) (bool, error) {
	return pk.verifySignature(msg, sig, b.dst)
}

// The AggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// Each message must be different or this will return false.
// See section 3.1.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigBasic) AggregateVerify(pks []*PublicKey, msgs [][]byte, sigs []*Signature) (bool, error) {
	if !allRowsUnique(msgs) {
		return false, fmt.Errorf("all messages must be distinct")
	}
	asig, err := aggregateSignatures(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, msgs, b.dst)
}

// SigAug is minimal-pubkey-size scheme that doesn't support FastAggregateVerificiation.
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.2
type SigAug struct {
	dst string
}

// Creates a new BLS message augmentation signature scheme with the standard domain separation tag used for signatures.
func NewSigAug() *SigAug {
	return &SigAug{dst: blsSignatureAugDst}
}

// Creates a new BLS message augmentation signature scheme with a custom domain separation tag used for signatures.
func NewSigAugWithDst(signDst string) *SigAug {
	return &SigAug{dst: signDst}
}

// Creates a new BLS key pair
func (b SigAug) Keygen() (*PublicKey, *SecretKey, error) {
	return generateKeys()
}

// Creates a new BLS secret key
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigAug) KeygenWithSeed(ikm []byte) (*PublicKey, *SecretKey, error) {
	return generateKeysWithSeed(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigAug) ThresholdKeygen(threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeys(threshold, total)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigAug) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeed(ikm, threshold, total)
}

// Computes a signature in G1 from sk, a secret key, and a message
// See section 3.2.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigAug) Sign(sk *SecretKey, msg []byte) (*Signature, error) {
	if len(msg) == 0 {
		return nil, fmt.Errorf("message cannot be empty or nil")
	}

	pk, err := sk.GetPublicKey()
	if err != nil {
		return nil, err
	}
	bytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("MarshalBinary failed")
	}
	bytes = append(bytes, msg...)
	return sk.createSignature(bytes, b.dst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigAug) PartialSign(sks *SecretKeyShare, pk *PublicKey, msg []byte) (*PartialSignature, error) {
	if len(msg) == 0 {
		return nil, fmt.Errorf("message cannot be empty or nil")
	}

	bytes, err := pk.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("MarshalBinary failed")
	}
	bytes = append(bytes, msg...)
	return sks.partialSign(bytes, b.dst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigAug) CombineSignatures(sigs ...*PartialSignature) (*Signature, error) {
	return combineSigs(sigs)
}

// Checks that a signature is valid for the message under the public key pk
// See section 3.2.2 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigAug) Verify(pk *PublicKey, msg []byte, sig *Signature) (bool, error) {
	bytes, err := pk.MarshalBinary()
	if err != nil {
		return false, err
	}
	bytes = append(bytes, msg...)
	return pk.verifySignature(bytes, sig, b.dst)
}

// The AggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// See section 3.2.3 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigAug) AggregateVerify(pks []*PublicKey, msgs [][]byte, sigs []*Signature) (bool, error) {
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
	asig, err := aggregateSignatures(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, data, b.dst)
}

// SigEth2 supports signatures on Eth2.
// Internally is an alias for SigPop
type SigEth2 = SigPop

// NewSigEth2 Creates a new BLS ETH2 signature scheme with the standard domain separation tag used for signatures.
func NewSigEth2() *SigEth2 {
	return NewSigPop()
}

// SigPop is minimal-pubkey-size scheme that supports FastAggregateVerification
// and requires using proofs of possession to mitigate rogue-key attacks
// see: https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03#section-4.2.3
type SigPop struct {
	sigDst string
	popDst string
}

// Creates a new BLS proof of possession signature scheme with the standard domain separation tag used for signatures.
func NewSigPop() *SigPop {
	return &SigPop{sigDst: blsSignaturePopDst, popDst: blsPopProofDst}
}

// Creates a new BLS message proof of possession signature scheme with a custom domain separation tag used for signatures.
func NewSigPopWithDst(signDst, popDst string) (*SigPop, error) {
	if signDst == popDst {
		return nil, fmt.Errorf("domain separation tags cannot be equal")
	}
	return &SigPop{sigDst: signDst, popDst: popDst}, nil
}

// Creates a new BLS key pair
func (b SigPop) Keygen() (*PublicKey, *SecretKey, error) {
	return generateKeys()
}

// Creates a new BLS secret key
// Input key material (ikm) MUST be at least 32 bytes long,
// but it MAY be longer.
func (b SigPop) KeygenWithSeed(ikm []byte) (*PublicKey, *SecretKey, error) {
	return generateKeysWithSeed(ikm)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigPop) ThresholdKeygen(threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeys(threshold, total)
}

// ThresholdKeyGen generates a public key and `total` secret key shares such that
// `threshold` of them can be combined in signatures
func (b SigPop) ThresholdKeygenWithSeed(ikm []byte, threshold, total uint) (*PublicKey, []*SecretKeyShare, error) {
	return thresholdGenerateKeysWithSeed(ikm, threshold, total)
}

// Computes a signature in G2 from sk, a secret key, and a message
// See section 2.6 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) Sign(sk *SecretKey, msg []byte) (*Signature, error) {
	return sk.createSignature(msg, b.sigDst)
}

// Compute a partial signature in G2 that can be combined with other partial signature
func (b SigPop) PartialSign(sks *SecretKeyShare, msg []byte) (*PartialSignature, error) {
	return sks.partialSign(msg, b.sigDst)
}

// CombineSignatures takes partial signatures to yield a completed signature
func (b SigPop) CombineSignatures(sigs ...*PartialSignature) (*Signature, error) {
	return combineSigs(sigs)
}

// Checks that a signature is valid for the message under the public key pk
// See section 2.7 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) Verify(pk *PublicKey, msg []byte, sig *Signature) (bool, error) {
	return pk.verifySignature(msg, sig, b.sigDst)
}

// The aggregateVerify algorithm checks an aggregated signature over
// several (PK, message, signature) pairs.
// Each message must be different or this will return false.
// See section 3.1.1 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) AggregateVerify(pks []*PublicKey, msgs [][]byte, sigs []*Signature) (bool, error) {
	if !allRowsUnique(msgs) {
		return false, fmt.Errorf("all messages must be distinct")
	}
	asig, err := aggregateSignatures(sigs...)
	if err != nil {
		return false, err
	}
	return asig.aggregateVerify(pks, msgs, b.sigDst)
}

// Combine many signatures together to form a Multisignature.
// Multisignatures can be created when multiple signers jointly
// generate signatures over the same message.
func (b SigPop) AggregateSignatures(sigs ...*Signature) (*MultiSignature, error) {
	g1, err := aggregateSignatures(sigs...)
	if err != nil {
		return nil, err
	}
	return &MultiSignature{value: g1.Value}, nil
}

// Combine many public keys together to form a Multipublickey.
// Multipublickeys are used to verify multisignatures.
func (b SigPop) AggregatePublicKeys(pks ...*PublicKey) (*MultiPublicKey, error) {
	g2, err := aggregatePublicKeys(pks...)
	if err != nil {
		return nil, err
	}
	return &MultiPublicKey{value: g2.value}, nil
}

// Checks that a multisignature is valid for the message under the multi public key
// Similar to FastAggregateVerify except the keys and signatures have already been
// combined. See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) VerifyMultiSignature(pk *MultiPublicKey, msg []byte, sig *MultiSignature) (bool, error) {
	s := &Signature{Value: sig.value}
	p := &PublicKey{value: pk.value}
	return p.verifySignature(msg, s, b.sigDst)
}

// FastAggregateVerify verifies an aggregated signature against the specified message and set of public keys.
// See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) FastAggregateVerify(pks []*PublicKey, msg []byte, asig *Signature) (bool, error) {
	apk, err := aggregatePublicKeys(pks...)
	if err != nil {
		return false, err
	}
	return apk.verifySignature(msg, asig, b.sigDst)
}

// FastAggregateVerifyConstituent aggregates all constituent signatures and the verifies
// them against the specified message and public keys
// See section 3.3.4 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) FastAggregateVerifyConstituent(pks []*PublicKey, msg []byte, sigs []*Signature) (bool, error) {
	// Aggregate the constituent signatures
	asig, err := aggregateSignatures(sigs...)
	if err != nil {
		return false, err
	}
	// And verify
	return b.FastAggregateVerify(pks, msg, asig)
}

// Create a proof of possession for the corresponding public key.
// A proof of possession must be created for each public key to be used
// in FastAggregateVerify or a Multipublickey to avoid rogue key attacks.
// See section 3.3.2 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) PopProve(sk *SecretKey) (*ProofOfPossession, error) {
	return sk.createProofOfPossession(b.popDst)
}

// verify a proof of possession for the corresponding public key is valid.
// A proof of possession must be created for each public key to be used
// in FastAggregateVerify or a Multipublickey to avoid rogue key attacks.
// See section 3.3.3 from
// https://tools.ietf.org/html/draft-irtf-cfrg-bls-signature-03
func (b SigPop) PopVerify(pk *PublicKey, pop2 *ProofOfPossession) (bool, error) {
	return pop2.verify(pk, b.popDst)
}
