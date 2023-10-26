package bls

import (
	"encoding/base64"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	bn256 "github.com/umbracle/go-eth-bn256"
)

const (
	PublicKeySize = 128
)

var (
	errInfinityPoint        = errors.New("infinity point")
	ErrInvalidPublicKeySize = fmt.Errorf("public key must be %d bytes long", PublicKeySize)
)

// PublicKey represents bls public key
type PublicKey struct {
	g2 *bn256.G2
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	return p.g2.Marshal()
}

// MarshalText implements the json.Marshaler interface.
func (p *PublicKey) MarshalText() ([]byte, error) {
	dst := base64.StdEncoding.EncodeToString(p.Marshal())

	return []byte(dst), nil
}

// UnmarshalText implements encoding.TextMarshaler interface
func (p *PublicKey) UnmarshalText(buf []byte) error {
	res, err := base64.StdEncoding.DecodeString(string(buf))
	if err != nil {
		return err
	}

	pub, err := UnmarshalPublicKey(res)
	if err != nil {
		return err
	}

	p.g2 = pub.g2

	return nil
}

// ToBigInt converts public key to 4 big ints
func (p *PublicKey) ToBigInt() [4]*big.Int {
	blsKey := p.Marshal()

	return [4]*big.Int{
		new(big.Int).SetBytes(blsKey[32:64]),
		new(big.Int).SetBytes(blsKey[0:32]),
		new(big.Int).SetBytes(blsKey[96:128]),
		new(big.Int).SetBytes(blsKey[64:96]),
	}
}

// UnmarshalPublicKey unmarshals bytes to public key
func UnmarshalPublicKey(data []byte) (*PublicKey, error) {
	if len(data) < PublicKeySize {
		return nil, ErrInvalidPublicKeySize
	}

	g2 := new(bn256.G2)

	if _, err := g2.Unmarshal(data); err != nil {
		return nil, err
	}

	// check if it is the point at infinity
	if g2.IsInfinity() {
		return nil, errInfinityPoint
	}

	// check if not part of the subgroup
	if !g2.InCorrectSubgroup() {
		return nil, fmt.Errorf("incorrect subgroup")
	}

	return &PublicKey{g2: g2}, nil
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

type PublicKeys []*PublicKey

// Aggregate aggregates all public keys into one
func (pks PublicKeys) Aggregate() *PublicKey {
	newp := new(bn256.G2)

	for _, x := range pks {
		newp.Add(newp, x.g2)
	}

	return &PublicKey{g2: newp}
}
