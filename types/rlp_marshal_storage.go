package types

import "github.com/umbracle/fastrlp"

type RLPStoreMarshaler interface {
	MarshalStoreRLPTo(dst []byte) []byte
}

func (b *Body) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(b.MarshalRLPWith, dst)
}

func (b *Body) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()

	// transactions
	txs := (Transactions)(b.Transactions)
	vv.Set(txs.MarshalStoreRLPWith(ar))

	// uncles
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

func (tt *Transactions) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	if len(*tt) == 0 {
		return a.NewNullArray()
	}

	v0 := a.NewArray()

	for _, tx := range *tt {
		if tx.IsTypedTransaction() {
			v0.Set(a.NewBytes([]byte{byte(tx.Type())}))
		}

		v0.Set(tx.MarshalStoreRLPWith(a))
	}

	return v0
}

func (t *Transaction) MarshalStoreRLPTo(dst []byte) []byte {
	if t.IsTypedTransaction() {
		dst = append(dst, byte(t.Type()))
	}

	return MarshalRLPTo(t.MarshalStoreRLPWith, dst)
}

func (t *Transaction) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	// Payload defines RLP encoding rule instead of transaction
	return t.Payload.MarshalStoreRLPWith(a)
}

func (t *LegacyTransaction) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	// consensus part
	vv.Set(t.MarshalRLPWith(a))

	// context part
	vv.Set(a.NewBytes(t.From.Bytes()))

	return vv
}

func (t *StateTransaction) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	// consensus part
	vv.Set(t.MarshalRLPWith(a))

	// context part

	return vv
}

func (r Receipts) MarshalStoreRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.MarshalStoreRLPWith, dst)
}

func (r *Receipts) MarshalStoreRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	for _, rr := range *r {
		if rr.IsTypedTransaction() {
			vv.Set(a.NewBytes([]byte{byte(rr.TransactionType)}))
		}

		vv.Set(rr.MarshalStoreRLPWith(a))
	}

	return vv
}

func (r *Receipt) MarshalStoreRLPTo(dst []byte) []byte {
	if r.IsTypedTransaction() {
		dst = append(dst, byte(r.TransactionType))
	}

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
