package archive

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/fastrlp"
)

// Metadata is the data stored in the beginning of backup
type Metadata struct {
	Latest     uint64
	LatestHash types.Hash
}

func (m *Metadata) MarshalRLP() []byte {
	return m.MarshalRLPTo(nil)
}

func (m *Metadata) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(m.MarshalRLPWith, dst)
}

func (m *Metadata) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(m.Latest))
	vv.Set(arena.NewBytes(m.LatestHash.Bytes()))

	return vv
}

func (m *Metadata) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(m.UnmarshalRLPFrom, input)
}

func (m *Metadata) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if num := len(elems); num != 2 {
		return fmt.Errorf("not enough elements to decode Metadata, expected 2 but found %d", num)
	}

	if m.Latest, err = elems[0].GetUint64(); err != nil {
		return err
	}
	if err = elems[1].GetHash(m.LatestHash[:]); err != nil {
		return err
	}

	return nil
}
