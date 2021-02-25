package storage

import (
	"github.com/0xPolygon/minimal/types"
	"github.com/umbracle/fastrlp"
)

type Forks []types.Hash

func (f *Forks) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(f, dst)
}

func (f *Forks) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	var vr *fastrlp.Value
	if len(*f) == 0 {
		vr = ar.NewNullArray()
	} else {
		vr = ar.NewArray()
		for _, fork := range *f {
			vr.Set(ar.NewCopyBytes(fork[:]))
		}
	}
	return vr
}

func (f *Forks) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(f, input)
}

func (f *Forks) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		panic(err)
	}
	forks := make([]types.Hash, len(elems))
	for indx, elem := range elems {
		if err := elem.GetHash(forks[indx][:]); err != nil {
			panic(err)
		}
	}
	*f = forks
	return nil
}
