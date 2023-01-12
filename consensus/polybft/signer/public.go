package bls

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/helper/common"
	ellipticcurve "github.com/consensys/gnark-crypto/ecc/bn254"
)

// PublicKey represents bls public key
type PublicKey struct {
	p *ellipticcurve.G2Affine
}

// aggregate adds the given public keys
func (p *PublicKey) aggregate(next *PublicKey) *PublicKey {
	newp := new(ellipticcurve.G2Affine)

	if p.p != nil {
		if next.p != nil {
			newp = newp.Add(p.p, next.p)
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

	var b bytes.Buffer

	if err := ellipticcurve.NewEncoder(&b, ellipticcurve.RawEncoding()).Encode(p.p); err != nil {
		panic(err) // TODO:
	}

	return b.Bytes()
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Marshal())
}

// UnmarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) UnmarshalJSON(b []byte) error {
	var d []byte

	if err := json.Unmarshal(b, &d); err != nil {
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

	output := new(ellipticcurve.G2Affine)
	decoder := ellipticcurve.NewDecoder(bytes.NewReader(raw))

	if err := decoder.Decode(output); err != nil {
		return nil, err
	}

	return &PublicKey{p: output}, nil
}

// ToBigInt converts public key to 4 big ints
func (p PublicKey) ToBigInt() [4]*big.Int {
	bytes := p.Marshal()

	res := [4]*big.Int{
		new(big.Int).SetBytes(bytes[32:64]),
		new(big.Int).SetBytes(bytes[0:32]),
		new(big.Int).SetBytes(bytes[96:128]),
		new(big.Int).SetBytes(bytes[64:96]),
	}

	return res
}

func (p PublicKey) String() string {
	return fmt.Sprintf("(%s %s, %s %s)",
		p.p.X.A0.String(), p.p.X.A1.String(),
		p.p.Y.A0.String(), p.p.Y.A1.String())
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
	newp := new(ellipticcurve.G2Jac)

	for _, x := range pubs {
		if x.p != nil {
			newp.AddMixed(x.p)
		}
	}

	return &PublicKey{p: new(ellipticcurve.G2Affine).FromJacobian(newp)}
}
