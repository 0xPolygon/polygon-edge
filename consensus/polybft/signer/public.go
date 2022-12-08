package bls

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/kilic/bn254"
)

// PublicKey represents bls public key
type PublicKey struct {
	p *bn254.PointG2
}

// aggregate adds the given public keys
func (p *PublicKey) aggregate(next *PublicKey) *PublicKey {
	g := bn254.NewG2()

	newp := new(bn254.PointG2)
	newp.Zero()

	if p.p != nil {
		if next.p != nil {
			g.Add(newp, p.p, next.p)
		} else {
			newp.Set(p.p)
		}
	} else if next.p != nil {
		newp.Set(next.p)
	}

	return &PublicKey{p: newp}
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	if p.p == nil {
		return nil
	}

	return bn254.NewG2().ToBytes(p.p)
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Marshal())
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

	p, err := bn254.NewG2().FromBytes(raw)
	if err != nil {
		return nil, err
	}

	return &PublicKey{p: p}, nil
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
	g, newp := bn254.NewG2(), new(bn254.PointG2)

	newp.Set(g.Zero())

	for _, x := range pubs {
		if x.p != nil {
			g.Add(newp, newp, x.p)
		}
	}

	return &PublicKey{p: newp}
}
