package bls

import (
	"bytes"
	"errors"
	"math/big"

	ellipticcurve "github.com/consensys/gnark-crypto/ecc/bn254"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	p *ellipticcurve.G1Affine
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	/*
		e := bn254.NewEngine()
		messagePoint, err := e.G1.HashToCurveFT(message, GetDomain())
		if err != nil {
			return false
		}
		e.AddPair(messagePoint, publicKey.p)
		e.AddPairInv(s.p, e.G2.One())
		return e.Check()
		func (e *Engine) AddPair(g1 *PointG1, g2 *PointG2) *Engine {
			return e.addPair(e.G1.New().Set(g1), e.G2.New().Set(g2))
		}

		// AddPairInv adds a G1, G2 point pair to pairing engine. G1 point is negated.
		func (e *Engine) AddPairInv(g1 *PointG1, g2 *PointG2) *Engine {
			ng1 := e.G1.New().Set(g1)
			e.G1.Neg(ng1, ng1)
			return e.addPair(ng1, e.G2.New().Set(g2))
		}

		func (e *Engine) addPair(g1 *PointG1, g2 *PointG2) *Engine {
			p := newPair(g1, g2)
			if !e.isZero(p) {
				e.affine(p)
				e.pairs = append(e.pairs, p)
			}
			return e
		}
	*/
	messagePoint, err := ellipticcurve.HashToG1(message, GetDomain())
	if err != nil {
		return false
	}

	sigInv := ellipticcurve.G1Affine{}
	sigInv.Neg(s.p)

	result, err := ellipticcurve.PairingCheck(
		[]ellipticcurve.G1Affine{messagePoint, sigInv},
		[]ellipticcurve.G2Affine{*publicKey.p, *ellipticCurveG2})

	return err == nil && result
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(next *Signature) *Signature {
	newp := new(ellipticcurve.G1Affine)

	if s.p != nil {
		if next.p != nil {
			newp = newp.Add(s.p, next.p)
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

	var b bytes.Buffer

	if err := ellipticcurve.NewEncoder(&b, ellipticcurve.RawEncoding()).Encode(s.p); err != nil {
		return nil, err
	}

	return b.Bytes(), nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal signature from empty slice")
	}

	output := new(ellipticcurve.G1Affine)
	decoder := ellipticcurve.NewDecoder(bytes.NewReader(raw))

	if err := decoder.Decode(output); err != nil {
		return nil, err
	}

	return &Signature{p: output}, nil
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
	newp := new(ellipticcurve.G1Jac)

	for _, x := range s {
		if x.p != nil {
			newp.AddMixed(x.p)
		}
	}

	return &Signature{p: new(ellipticcurve.G1Affine).FromJacobian(newp)}
}
