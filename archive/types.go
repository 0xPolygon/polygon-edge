package archive

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

// Metadata is the data stored in the beginning of backup
type Metadata struct {
	Latest     uint64
	LatestHash types.Hash
}

// MarshalRLP returns RLP encoded bytes
func (m *Metadata) MarshalRLP() []byte {
	return m.MarshalRLPTo(nil)
}

// MarshalRLPTo sets RLP encoded bytes to given byte slice
func (m *Metadata) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(m.MarshalRLPWith, dst)
}

// MarshalRLPWith appends own field into arena for encode
func (m *Metadata) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(m.Latest))
	vv.Set(arena.NewBytes(m.LatestHash.Bytes()))

	return vv
}

// UnmarshalRLP unmarshals and sets the fields from RLP encoded bytes
func (m *Metadata) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(m.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom sets the fields from parsed RLP encoded value
func (m *Metadata) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	if len(elems) < 2 {
		return fmt.Errorf("incorrect number of elements to decode Metadata, expected 2 but found %d", len(elems))
	}

	if m.Latest, err = elems[0].GetUint64(); err != nil {
		return err
	}

	if err = elems[1].GetHash(m.LatestHash[:]); err != nil {
		return err
	}

	return nil
}
