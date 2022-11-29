package bls

import (
	"errors"
	"math/big"

	"github.com/kilic/bn254"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	p *bn254.PointG1
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	e := bn254.NewEngine()
	messagePoint, err := e.G1.HashToCurveFT(message, GetDomain())

	if err != nil {
		return false
	}

	e.AddPair(messagePoint, publicKey.p)
	e.AddPairInv(s.p, e.G2.One())

	return e.Check()
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(next *Signature) *Signature {
	g := bn254.NewG1()

	newp := new(bn254.PointG1)
	newp.Zero()

	if s.p != nil {
		if next.p != nil {
			g.Add(newp, s.p, next.p)
		} else {
			newp.Set(s.p)
		}
	} else if next.p != nil {
		newp.Set(next.p)
	}

	return &Signature{p: newp}
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	if s.p == nil {
		return nil, errors.New("cannot marshal empty signature")
	}

	return bn254.NewG1().ToBytes(s.p), nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal signature from empty slice")
	}

	p, err := bn254.NewG1().FromBytes(raw)
	if err != nil {
		return nil, err
	}

	return &Signature{p: p}, nil
}

// ToBigInt marshalls signature (which is point) to 2 big ints - one for each coordinate
func (s Signature) ToBigInt() ([2]*big.Int, error) {
	sig, err := s.Marshal()
	if err != nil {
		return [2]*big.Int{}, err
	}

	res := [2]*big.Int{
		new(big.Int).SetBytes(sig[0:32]),
		new(big.Int).SetBytes(sig[32:64]),
	}

	return res, nil
}

// Signatures is a slice of signatures
type Signatures []*Signature

// Aggregate sums the given array of signatures
func (s Signatures) Aggregate() *Signature {
	g, newp := bn254.NewG1(), new(bn254.PointG1)

	newp.Set(g.Zero())

	for _, x := range s {
		if x.p != nil {
			g.Add(newp, newp, x.p)
		}
	}

	return &Signature{p: newp}
}
