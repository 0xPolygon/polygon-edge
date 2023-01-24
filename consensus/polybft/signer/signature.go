package bls

import (
	"errors"
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	g1 *bn256.G1
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(pub *PublicKey, message []byte) bool {
	point, err := hashToPoint(message, domain)
	if err != nil {
		return false
	}

	return bn256.PairingCheck([]*bn256.G1{s.g1, point}, []*bn256.G2{negG2Point, pub.g2})
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(next *Signature) *Signature {
	newp := new(bn256.G1)

	if s.g1 != nil {
		if next.g1 != nil {
			newp.Add(s.g1, next.g1)
		} else {
			newp.Set(s.g1)
		}
	} else if next.g1 != nil {
		newp.Set(next.g1)
	}

	return &Signature{g1: newp}
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	if s.g1 == nil {
		return nil, errors.New("cannot marshal empty signature")
	}

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
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal signature from empty slice")
	}

	g1 := new(bn256.G1)

	if _, err := g1.Unmarshal(raw); err != nil {
		return nil, err
	}

	return &Signature{g1: g1}, nil
}

// Signatures is a slice of signatures
type Signatures []*Signature

// Aggregate sums the given array of signatures
func (sigs Signatures) Aggregate() *Signature {
	g1 := new(bn256.G1)

	for _, sig := range sigs {
		if sig.g1 != nil {
			g1.Add(g1, sig.g1)
		}
	}

	return &Signature{g1: g1}
}
