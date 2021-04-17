package types

import "github.com/umbracle/fastrlp"

type RLPStoreMarshaler interface {
	MarshalStoreRLPTo(dst []byte) []byte
}

func (r Receipts) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.MarshalStoreRLPWith, dst)
}

func (r *Receipts) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()
	for _, rr := range *r {
		vv.Set(rr.MarshalStoreRLPWith(a))
	}
	return vv
}

func (r *Receipt) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.MarshalStoreRLPWith, dst)
}

func (r *Receipt) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	// use the hash part
	vv := a.NewArray()
	vv.Set(r.MarshalRLPWith(a))

	if r.ContractAddress == ZeroAddress {
		vv.Set(a.NewNull())
	} else {
		vv.Set(a.NewBytes(r.ContractAddress.Bytes()))
	}

	// gas used
	vv.Set(a.NewUint(r.GasUsed))
	return vv
}
