package bls

import (
	"encoding/json"
	"errors"
	"math/big"
)

// PublicKey represents bls public key
type PublicKey struct {
	p *G2
}

// aggregate adds the given public keys
func (p *PublicKey) aggregate(next *PublicKey) *PublicKey {
	newp := new(G2)

	if p.p != nil {
		if next.p != nil {
			G2Add(newp, p.p, next.p)
		} else {
			G2Add(newp, newp, p.p)
		}
	} else if next.p != nil {
		G2Add(newp, newp, next.p)
	}

	return &PublicKey{p: newp}
}

// Marshal marshal the key to bytes.
func (p *PublicKey) Marshal() []byte {
	if p.p == nil {
		return nil
	}

	return G2ToBytes(p.p)
}

// MarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.Marshal())
}

// UnmarshalJSON implements the json.Marshaler interface.
func (p *PublicKey) UnmarshalJSON(raw []byte) error {
	var jsonBytes []byte

	if err := json.Unmarshal(raw, &jsonBytes); err != nil {
		return err
	}

	bigInts, err := BytesToBigInt4(jsonBytes)
	if err != nil {
		return err
	}

	p.p, err = G2FromBigInt(bigInts)
	if err != nil {
		return err
	}

	return nil
}

// UnmarshalPublicKey reads the public key from the given byte array
func UnmarshalPublicKey(raw []byte) (*PublicKey, error) {
	if len(raw) == 0 {
		return nil, errors.New("cannot unmarshal public key from empty slice")
	}

	bigInts, err := BytesToBigInt4(raw)
	if err != nil {
		return nil, err
	}

	g2, err := G2FromBigInt(bigInts)
	if err != nil {
		return nil, err
	}

	return &PublicKey{p: g2}, nil
}

// ToBigInt converts public key to 4 big ints
func (p PublicKey) ToBigInt() [4]*big.Int {
	return G2ToBigInt(p.p)
}

// UnmarshalPublicKeyFromBigInt unmarshals public key from 4 big ints
// Order of coordinates is [A.Y, A.X, B.Y, B.X]
func UnmarshalPublicKeyFromBigInt(b [4]*big.Int) (*PublicKey, error) {
	g2, err := G2FromBigInt(b)
	if err != nil {
		return nil, err
	}

	return &PublicKey{p: g2}, nil
}

// aggregatePublicKeys calculates P1 + P2 + ...
func aggregatePublicKeys(pubs []*PublicKey) *PublicKey {
	newp := new(G2)

	for _, x := range pubs {
		if x.p != nil {
			G2Add(newp, newp, x.p)
		}
	}

	return &PublicKey{p: newp}
}
