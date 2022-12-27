package bls

import (
	"errors"
	"math/big"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	p *G1
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	messagePoint, err := HashToG1(message)
	if err != nil {
		return false
	}

	e1, e2 := new(GT), new(GT)

	G1Neg(messagePoint, messagePoint)
	PrecomputedMillerLoop(e1, s.p, GetCoef())
	MillerLoop(e2, messagePoint, publicKey.p)
	GTMul(e1, e1, e2)
	FinalExp(e1, e1)

	return e1.IsOne()
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(next *Signature) *Signature {
	newp := new(G1)

	if s.p != nil {
		if next.p != nil {
			G1Add(newp, s.p, next.p)
		} else {
			G1Add(newp, newp, s.p)
		}
	} else if next.p != nil {
		G1Add(newp, newp, next.p)
	}

	return &Signature{p: newp}
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	if s.p == nil {
		return nil, errors.New("cannot marshal empty signature")
	}

	return G1ToBytes(s.p), nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal signature from empty slice")
	}

	bigInts, err := BytesToBigInt2(raw)
	if err != nil {
		return nil, err
	}

	g1, err := G1FromBigInt(bigInts)
	if err != nil {
		return nil, err
	}

	return &Signature{p: g1}, nil
}

// ToBigInt marshalls signature (which is point) to 2 big ints - one for each coordinate
func (s Signature) ToBigInt() ([2]*big.Int, error) {
	return G1ToBigInt(s.p), nil
}

// Signatures is a slice of signatures
type Signatures []*Signature

// Aggregate sums the given array of signatures
func (s Signatures) Aggregate() *Signature {
	newp := new(G1)

	for _, x := range s {
		if x.p != nil {
			G1Add(newp, newp, x.p)
		}
	}

	return &Signature{p: newp}
}
