package polybft

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Proposal struct {
	Block *types.Block
	Hash  types.Hash
}

func (p *Proposal) MarshalRLP() []byte {
	return types.MarshalRLPTo(p.MarshalRLPWith, nil)
}

func (p *Proposal) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(p.MarshalRLPWith, dst)
}

// MarshalRLPWith marshals the header to RLP with a specific fastrlp.Arena
func (p *Proposal) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(p.Block.MarshalRLPWith(arena))
	vv.Set(arena.NewBytes(p.Hash.Bytes()))

	return vv
}

func (p *Proposal) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(p.UnmarshalRLPFrom, input)
}

func (p *Proposal) UnmarshalRLPFrom(parser *fastrlp.Parser, value *fastrlp.Value) error {
	elems, err := value.GetElems()
	if err != nil {
		return err
	}

	if num := len(elems); num < 2 {
		return fmt.Errorf("incorrect number of elements to decode block, expected 2 but found %d", num)
	}

	// block
	p.Block = &types.Block{}
	if err := p.Block.UnmarshalRLPFrom(parser, elems[0]); err != nil {
		return err
	}

	// receiptroot
	if err = elems[1].GetHash(p.Hash[:]); err != nil {
		return err
	}

	return nil
}
