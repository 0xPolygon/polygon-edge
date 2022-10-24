package bls

import (
	"errors"
	"math/big"

	bn256 "github.com/umbracle/go-eth-bn256"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	p *bn256.G1
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	hashPoint, err := g1HashToPoint(message)
	if err != nil {
		// this should never happen, probably need a log here
		return false
	}

	a := []*bn256.G1{new(bn256.G1).Neg(s.p), hashPoint}
	b := []*bn256.G2{&g2, publicKey.p}

	return bn256.PairingCheck(a, b)
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(onemore *Signature) *Signature {
	var p *bn256.G1
	if s.p == nil {
		p = new(bn256.G1).Set(&zeroG1)
	} else {
		p = new(bn256.G1).Set(s.p)
	}

	p.Add(p, onemore.p)

	return &Signature{p: p}
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	if s.p == nil {
		return nil, errors.New("cannot marshal empty signature")
	}

	return s.p.Marshal(), nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal signature from empty slice")
	}

	p := new(bn256.G1)
	_, err := p.Unmarshal(raw)

	return &Signature{p: p}, err
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
	p := *new(bn256.G1).Set(&zeroG1)
	for _, sig := range s {
		point := new(bn256.G1).Set(sig.p)
		p.Add(&p, point)
	}

	return &Signature{p: &p}
}
