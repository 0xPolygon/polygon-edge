package types

import (
	"fmt"

	"github.com/umbracle/fastrlp"
)

type RLPStoreUnmarshaler interface {
	UnmarshalStoreRLP(input []byte) error
}

func (r *Receipts) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalStoreRLPFrom, input)
}

func (r *Receipts) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	for _, elem := range elems {
		rr := &Receipt{}
		if err := rr.UnmarshalStoreRLPFrom(p, elem); err != nil {
			return err
		}
		(*r) = append(*r, rr)
	}
	return nil
}

func (r *Receipt) UnmarshalStoreRLP(input []byte) error {
	return UnmarshalRlp(r.UnmarshalStoreRLPFrom, input)
}

func (r *Receipt) UnmarshalStoreRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if len(elems) != 3 {
		return fmt.Errorf("expected 3 elements")
	}

	if err := r.UnmarshalRLPFrom(p, elems[0]); err != nil {
		return err
	}

	{
		// contract address
		vv, err := elems[1].Bytes()
		if err != nil {
			return err
		}
		if len(vv) == 20 {
			// address
			r.ContractAddress = BytesToAddress(vv)
		}
	}

	// gas used
	if r.GasUsed, err = elems[2].GetUint64(); err != nil {
		return err
	}
	return nil
}
