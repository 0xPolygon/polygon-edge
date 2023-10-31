package bls

import (
	"fmt"
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

const (
	SignatureSize = 64
)

var (
	ErrInvalidSignatureSize = fmt.Errorf("signature must be %d bytes long", SignatureSize)
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	g1 *bn256.G1
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(pub *PublicKey, message, domain []byte) bool {
	point, err := hashToPoint(message, domain)
	if err != nil {
		return false
	}

	return bn256.PairingCheck([]*bn256.G1{s.g1, point}, []*bn256.G2{negG2Point, pub.g2})
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg, domain []byte) bool {
	return s.Verify(PublicKeys(publicKeys).Aggregate(), msg, domain)
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	return s.g1.Marshal(), nil
}

// ToBigInt marshalls signature (which is point) to 2 big ints - one for each coordinate
func (s Signature) ToBigInt() ([2]*big.Int, error) {
	sig, err := s.Marshal()
	if err != nil {
		return [2]*big.Int{}, err
	}

	return [2]*big.Int{
		new(big.Int).SetBytes(sig[0:32]),
		new(big.Int).SetBytes(sig[32:64]),
	}, nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	if len(raw) < SignatureSize {
		return nil, ErrInvalidSignatureSize
	}

	g1 := new(bn256.G1)
	if _, err := g1.Unmarshal(raw); err != nil {
		return nil, err
	}

	// check if it is the point at infinity
	if g1.IsInfinity() {
		return nil, errInfinityPoint
	}

	// check if not part of the subgroup
	if !g1.InCorrectSubgroup() {
		return nil, fmt.Errorf("incorrect subgroup")
	}

	return &Signature{g1: g1}, nil
}

// Signatures is a slice of signatures
type Signatures []*Signature

// Aggregate aggregates all signatures into one
func (sigs Signatures) Aggregate() *Signature {
	g1 := new(bn256.G1)

	for _, sig := range sigs {
		g1.Add(g1, sig.g1)
	}

	return &Signature{g1: g1}
}
