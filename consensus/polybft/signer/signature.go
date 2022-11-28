package bls

import (
	"errors"
	"math/big"

	"github.com/prysmaticlabs/go-bls"
)

// Signature represents bls signature which is point on the curve
type Signature struct {
	sig *bls.Sign
}

// Verify checks the BLS signature of the message against the public key of its signer
func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	return s.sig.Verify(publicKey.p, message)
}

// VerifyAggregated checks the BLS signature of the message against the aggregated public keys of its signers
func (s *Signature) VerifyAggregated(publicKeys []*PublicKey, msg []byte) bool {
	aggPubs := aggregatePublicKeys(publicKeys)

	return s.Verify(aggPubs, msg)
}

// Aggregate adds the given signatures
func (s *Signature) Aggregate(onemore *Signature) {
	s.sig.Add(onemore.sig)
}

// Marshal the signature to bytes.
func (s *Signature) Marshal() ([]byte, error) {
	if s.sig == nil {
		return nil, errors.New("cannot marshal empty signature")
	}

	return s.sig.Serialize(), nil
}

// UnmarshalSignature reads the signature from the given byte array
func UnmarshalSignature(raw []byte) (*Signature, error) {
	sig := &Signature{}
	if err := sig.sig.Deserialize(raw); err != nil {
		return nil, err
	}

	return sig, nil
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
	firstSig := s[0]
	for i := 1; i < len(s); i++ {
		firstSig.sig.Add(s[i].sig)
	}

	return firstSig
}
