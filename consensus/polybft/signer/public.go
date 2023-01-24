package bls

import (
	"encoding/json"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bn256 "github.com/umbracle/go-eth-bn256"
)

// PublicKey represents bls public key
type PublicKey struct {
	g2 *bn256.G2
}

// Aggregate aggregates the given public key with current one
func (p *PublicKey) Aggregate(next *PublicKey) *PublicKey {
	newp := new(bn256.G2)

	if p.g2 != nil {
		if next.g2 != nil {
			newp.Add(p.g2, next.g2)
		} else {
			newp.Set(p.g2)
		}
	} else if next.g2 != nil {
		newp.Set(next.g2)
	}

	return &PublicKey{g2: newp}
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	if p.g2 == nil {
		return nil
	}

	return p.g2.Marshal()
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Marshal())
}

// UnmarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) UnmarshalJSON(jsonBytes []byte) error {
	var bytes []byte

	err := json.Unmarshal(jsonBytes, &bytes)
	if err != nil {
		return err
	}

	g2 := new(bn256.G2)
	if _, err := g2.Unmarshal(bytes); err != nil {
		return err
	}

	p.g2 = g2

	return nil
}

// ToBigInt converts public key to 4 big ints
func (p *PublicKey) ToBigInt() [4]*big.Int {
	blsKey := p.Marshal()
	if len(blsKey) != 128 {
		return [4]*big.Int{}
	}

	return [4]*big.Int{
		new(big.Int).SetBytes(blsKey[32:64]),
		new(big.Int).SetBytes(blsKey[0:32]),
		new(big.Int).SetBytes(blsKey[96:128]),
		new(big.Int).SetBytes(blsKey[64:96]),
	}
}

// UnmarshalPublicKey unmarshals bytes to public key
func UnmarshalPublicKey(data []byte) (*PublicKey, error) {
	g2 := new(bn256.G2)

	if _, err := g2.Unmarshal(data); err != nil {
		return nil, err
	}

	return &PublicKey{g2: g2}, nil
}

// UnmarshalPublicKeyFromBigInt unmarshals public key from 4 big ints
// Order of coordinates is [A.Y, A.X, B.Y, B.X]
func UnmarshalPublicKeyFromBigInt(b [4]*big.Int) (*PublicKey, error) {
	const size = 32

	var pubKeyBuf [size * 4]byte

	copy(pubKeyBuf[:], common.PadLeftOrTrim(b[1].Bytes(), size))
	copy(pubKeyBuf[size:], common.PadLeftOrTrim(b[0].Bytes(), size))
	copy(pubKeyBuf[size*2:], common.PadLeftOrTrim(b[3].Bytes(), size))
	copy(pubKeyBuf[size*3:], common.PadLeftOrTrim(b[2].Bytes(), size))

	return UnmarshalPublicKey(pubKeyBuf[:])
}

// aggregatePublicKeys calculates P1 + P2 + ...
func aggregatePublicKeys(pubs []*PublicKey) *PublicKey {
	newp := new(bn256.G2)

	for _, x := range pubs {
		if x.g2 != nil {
			newp.Add(newp, x.g2)
		}
	}

	return &PublicKey{g2: newp}
}
