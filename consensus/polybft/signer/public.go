package bls

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/prysmaticlabs/go-bls"
)

// PublicKey represents bls public key
type PublicKey struct {
	p *bls.PublicKey
}

// aggregate adds the given public keys
func (p *PublicKey) aggregate(onemore *PublicKey) {
	p.p.Add(onemore.p)
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	return p.p.Serialize()
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.p.Serialize())
}

// UnmarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) UnmarshalJSON(b []byte) error {
	var d []byte

	err := json.Unmarshal(b, &d)
	if err != nil {
		return err
	}

	pk, err := UnmarshalPublicKey(d)
	if err != nil {
		return err
	}

	p.p = pk.p

	return nil
}

// UnmarshalPublicKey reads the public key from the given byte array
func UnmarshalPublicKey(raw []byte) (*PublicKey, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal public key from empty slice")
	}

	publicKey := &PublicKey{}
	if err := publicKey.p.Deserialize(raw); err != nil {
		return nil, err
	}

	return publicKey, nil
}

// ToBigInt converts public key to 4 big ints
func (p PublicKey) ToBigInt() [4]*big.Int {
	blsKey := p.Marshal()
	res := [4]*big.Int{
		new(big.Int).SetBytes(blsKey[32:64]),
		new(big.Int).SetBytes(blsKey[0:32]),
		new(big.Int).SetBytes(blsKey[96:128]),
		new(big.Int).SetBytes(blsKey[64:96]),
	}

	return res
}

// UnmarshalPublicKeyFromBigInt unmarshals public key from 4 big ints
// Order of coordinates is [A.Y, A.X, B.Y, B.X]
func UnmarshalPublicKeyFromBigInt(b [4]*big.Int) (*PublicKey, error) {
	const size = 32

	var pubKeyBuf []byte

	pt1 := common.PadLeftOrTrim(b[1].Bytes(), size)
	pt2 := common.PadLeftOrTrim(b[0].Bytes(), size)
	pt3 := common.PadLeftOrTrim(b[3].Bytes(), size)
	pt4 := common.PadLeftOrTrim(b[2].Bytes(), size)

	pubKeyBuf = append(pubKeyBuf, pt1...)
	pubKeyBuf = append(pubKeyBuf, pt2...)
	pubKeyBuf = append(pubKeyBuf, pt3...)
	pubKeyBuf = append(pubKeyBuf, pt4...)

	return UnmarshalPublicKey(pubKeyBuf)
}

// aggregatePublicKeys calculates P1 + P2 + ...
func aggregatePublicKeys(pubs []*PublicKey) *PublicKey {
	firstKey := pubs[0]
	for i := 1; i < len(pubs); i++ {
		firstKey.aggregate(pubs[i])
	}

	return firstKey
}
