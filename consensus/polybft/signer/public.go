package bls

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bn254 "github.com/kilic/bn254"
	kilic "github.com/kilic/bn254/bls"
)

// PublicKey represents bls public key
type PublicKey struct {
	p *kilic.PublicKey
}

// aggregate adds the given public keys
func (p *PublicKey) aggregate(onemore *PublicKey) *PublicKey {
	// var agg *kilic.PublicKey
	// if p.p == nil {
	// 	agg = new(kilic.G2).Set(&zeroG2)
	// } else {
	// 	agg = new(kilic.G2).Set(p.p)
	// }

	// g2.Add(g2, onemore.p)

	// return &PublicKey{p: agg}

	g := bn254.NewG2()

	if p.p == nil {
		agg = new(kilic.PublicKey).Set(&zeroG2)
	} else {
		agg = new(kilic.G2).Set(p.p)
	}

	g := bn254.NewG2()
	if len(keys) == 0 {
		return &AggregatedKey{g.Zero()}
	}
	
	 := new(PointG2).Set(keys[0].point)
	for i := 1; i < len(keys); i++ {
		g.Add(aggregated, aggregated, keys[i].point)
	}
	return &AggregatedKey{aggregated}
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	return p.p.ToBytes()
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.p.ToBytes())
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

	p, err := kilic.PublicKeyFromBytes(raw)
	if err != nil {
		return nil, err
	}

	return &PublicKey{p: p}, err
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
	res := *new(kilic.G2).Set(&zeroG2)
	for i := 0; i < len(pubs); i++ {
		res.Add(&res, pubs[i].p)
	}

	return &PublicKey{p: &res}
}
