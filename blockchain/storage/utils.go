package storage

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/fastrlp"
)

type Forks []types.Hash

// MarshalRLPTo is a wrapper function for calling the type marshal implementation
func (f *Forks) MarshalRLPTo(dst []byte) []byte {
	return types.MarshalRLPTo(f.MarshalRLPWith, dst)
}

// MarshalRLPWith is the actual RLP marshal implementation for the type
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

// UnmarshalRLP is a wrapper function for calling the type unmarshal implementation
func (f *Forks) UnmarshalRLP(input []byte) error {
	return types.UnmarshalRlp(f.UnmarshalRLPFrom, input)
}

// UnmarshalRLPFrom is the actual RLP unmarshal implementation for the type
func (f *Forks) UnmarshalRLPFrom(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}

	forks := make([]types.Hash, len(elems))
	for indx, elem := range elems {
		if err := elem.GetHash(forks[indx][:]); err != nil {
			return err
		}
	}

	*f = forks

	return nil
}
