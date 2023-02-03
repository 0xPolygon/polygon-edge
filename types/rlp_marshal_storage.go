package types

import (
	"github.com/umbracle/fastrlp"
)

type RLPStoreMarshaler interface {
	MarshalStoreRLPTo(dst []byte) []byte
}

func (b *Body) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(b.marshalRLPWith, dst)
}

func (b *Body) marshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			v0.Set(tx.marshalStoreRLPWith(ar))
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
	return MarshalRLPTo(t.marshalStoreRLPWith, dst)
}

func (t *Transaction) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	if t.Type != LegacyTx {
		vv.Set(a.NewBytes([]byte{byte(t.Type)}))
	}

	// consensus part
	vv.Set(t.MarshalRLPWith(a))
	// context part
	vv.Set(a.NewBytes(t.From.Bytes()))

	return vv
}

func (r Receipts) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.marshalStoreRLPWith, dst)
}

func (r *Receipts) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()
	for _, rr := range *r {
		vv.Set(rr.marshalStoreRLPWith(a))
	}

	return vv
}

func (r *Receipt) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.marshalStoreRLPWith, dst)
}

func (r *Receipt) marshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	// use the hash part
	vv := a.NewArray()

	if !r.IsLegacyTx() {
		vv.Set(a.NewBytes([]byte{byte(r.TransactionType)}))
	}

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
