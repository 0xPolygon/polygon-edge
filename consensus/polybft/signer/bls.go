package bls

import (
	"encoding/hex"
	"math/big"

	bnsnark1 "github.com/0xPolygon/bnsnark1/core"
)

var domain, _ = hex.DecodeString("508e30424791cb9a71683381558c3da1979b6fa423b2d6db1396b1d94d7c4a78")

type PrivateKey struct {
	*bnsnark1.PrivateKey
}

type PublicKey struct {
	*bnsnark1.PublicKey
}

type Signature struct {
	*bnsnark1.Signature
}

type Signatures []*Signature

func (s Signatures) Aggregate() *Signature {
	oSignatures := make([]*bnsnark1.Signature, len(s))

	for i, v := range s {
		oSignatures[i] = v.Signature
	}

	oSignature := bnsnark1.AggregateSignatures(oSignatures)

	return &Signature{oSignature}
}

func (s *Signature) Verify(publicKey *PublicKey, message []byte) bool {
	return s.Signature.Verify(publicKey.PublicKey, message)
}

func (s *Signature) VerifyAggregated(pubs []*PublicKey, msg []byte) bool {
	oPubs := make([]*bnsnark1.PublicKey, len(pubs))

	for i, v := range pubs {
		oPubs[i] = v.PublicKey
	}

	return s.Signature.VerifyAggregated(oPubs, msg)
}

func (s *Signature) Aggregate(next *Signature) *Signature {
	if s.Signature == nil {
		s.Signature = &bnsnark1.Signature{}
	}

	sign := s.Signature.Aggregate(next.Signature)

	return &Signature{sign}
}

func (s *Signature) ToBigInt() ([2]*big.Int, error) {
	bytes, err := s.Marshal()
	if err != nil {
		return [2]*big.Int{}, err
	}

	return bytesToBigInt2(bytes)
}

func (p *PrivateKey) Sign(message []byte) (*Signature, error) {
	s, err := p.PrivateKey.Sign(message)
	if err != nil {
		return nil, err
	}

	return &Signature{s}, nil
}

func (p *PrivateKey) PublicKey() *PublicKey {
	return &PublicKey{p.PrivateKey.PublicKey()}
}

func (p *PublicKey) ToBigInt() [4]*big.Int {
	result, _ := bytesToBigInt4(p.Marshal())

	return result
}

// UnmarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) UnmarshalJSON(raw []byte) error {
	p.PublicKey = &bnsnark1.PublicKey{}

	return p.PublicKey.UnmarshalJSON(raw)
}

func UnmarshalSignature(raw []byte) (*Signature, error) {
	oSignature, err := bnsnark1.UnmarshalSignature(raw)
	if err != nil {
		return nil, err
	}

	return &Signature{oSignature}, nil
}

func UnmarshalSignatureFromBigInt(b [2]*big.Int) (*Signature, error) {
	return UnmarshalSignature(bytesFromBigInt2(b))
}

func UnmarshalPublicKey(raw []byte) (*PublicKey, error) {
	oPublicKey, err := bnsnark1.UnmarshalPublicKey(raw)
	if err != nil {
		return nil, err
	}

	return &PublicKey{oPublicKey}, nil
}

func UnmarshalPublicKeyFromBigInt(b [4]*big.Int) (*PublicKey, error) {
	return UnmarshalPublicKey(bytesFromBigInt4(b))
}

func UnmarshalPrivateKey(raw []byte) (*PrivateKey, error) {
	oPrivateKey, err := bnsnark1.UnmarshalPrivateKey(raw)
	if err != nil {
		return nil, err
	}

	return &PrivateKey{oPrivateKey}, nil
}

func GenerateBlsKey() (*PrivateKey, error) {
	oPrivateKey, err := bnsnark1.GenerateBlsKey()
	if err != nil {
		return nil, err
	}

	return &PrivateKey{oPrivateKey}, nil
}

func CreateRandomBlsKeys(total int) ([]*PrivateKey, error) {
	oPrivateKeys, err := bnsnark1.CreateRandomBlsKeys(total)
	if err != nil {
		return nil, err
	}

	result := make([]*PrivateKey, len(oPrivateKeys))

	for i, v := range oPrivateKeys {
		result[i] = &PrivateKey{v}
	}

	return result, nil
}

func GetDomain() []byte {
	return domain
}

// MarshalMessageToBigInt marshalls message into two big ints
// first we must convert message bytes to point and than for each coordinate we create big int
func MarshalMessageToBigInt(message []byte) ([2]*big.Int, error) {
	bytes, err := bnsnark1.MarshalMessage(message)
	if err != nil {
		return [2]*big.Int{}, err
	}

	return bytesToBigInt2(bytes)
}

func init() {
	bnsnark1.SetDomain(domain)
}
