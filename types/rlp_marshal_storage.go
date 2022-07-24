package types

import (
	"github.com/umbracle/fastrlp"
)

type RLPStoreMarshaler interface {
	MarshalStoreRLPTo(dst []byte) []byte
}

func (b *Body) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(b.MarshalRLPWith, dst)
}

func (b *Body) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			v0.Set(tx.MarshalStoreRLPWith(ar))
		}
		vv.Set(v0)
	}

	if len(b.Uncles) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v1 := ar.NewArray()
		for _, uncle := range b.Uncles {
			v1.Set(uncle.MarshalRLPWith(ar))
		}
		vv.Set(v1)
	}

	return vv
}

func (t *Transaction) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(t.MarshalStoreRLPWith, dst)
}

func (t *Transaction) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()
	// consensus part
	vv.Set(t.MarshalRLPWith(a))
	// context part
	vv.Set(a.NewBytes(t.From.Bytes()))

	return vv
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

	if r.ContractAddress == nil {
		vv.Set(a.NewNull())
	} else {
		vv.Set(a.NewBytes(r.ContractAddress.Bytes()))
	}

	// gas used
	vv.Set(a.NewUint(r.GasUsed))

	// TxHash
	vv.Set(a.NewBytes(r.TxHash.Bytes()))

	return vv
}
