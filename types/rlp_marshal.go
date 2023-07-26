package types

import (
	"github.com/umbracle/fastrlp"
)

const (
	RLPSingleByteUpperLimit = 0x7f
)

type RLPMarshaler interface {
	MarshalRLPTo(dst []byte) []byte
}

type marshalRLPFunc func(ar *fastrlp.Arena) *fastrlp.Value

func MarshalRLPTo(obj marshalRLPFunc, dst []byte) []byte {
	ar := fastrlp.DefaultArenaPool.Get()
	dst = obj(ar).MarshalTo(dst)
	fastrlp.DefaultArenaPool.Put(ar)

	return dst
}

func (b *Block) MarshalRLP() []byte {
	return b.MarshalRLPTo(nil)
}

func (b *Block) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(b.MarshalRLPWith, dst)
}

func (b *Block) MarshalRLPWith(ar *fastrlp.Arena) *fastrlp.Value {
	vv := ar.NewArray()
	vv.Set(b.Header.MarshalRLPWith(ar))

	if len(b.Transactions) == 0 {
		vv.Set(ar.NewNullArray())
	} else {
		v0 := ar.NewArray()
		for _, tx := range b.Transactions {
			if tx.Type != LegacyTx {
				v0.Set(ar.NewCopyBytes([]byte{byte(tx.Type)}))
			}

			v0.Set(tx.MarshalRLPWith(ar))
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

func (h *Header) MarshalRLP() []byte {
	return h.MarshalRLPTo(nil)
}

func (h *Header) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(h.MarshalRLPWith, dst)
}

// MarshalRLPWith marshals the header to RLP with a specific fastrlp.Arena
func (h *Header) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewCopyBytes(h.ParentHash.Bytes()))
	vv.Set(arena.NewCopyBytes(h.Sha3Uncles.Bytes()))
	vv.Set(arena.NewCopyBytes(h.Miner[:]))
	vv.Set(arena.NewCopyBytes(h.StateRoot.Bytes()))
	vv.Set(arena.NewCopyBytes(h.TxRoot.Bytes()))
	vv.Set(arena.NewCopyBytes(h.ReceiptsRoot.Bytes()))
	vv.Set(arena.NewCopyBytes(h.LogsBloom[:]))

	vv.Set(arena.NewUint(h.Difficulty))
	vv.Set(arena.NewUint(h.Number))
	vv.Set(arena.NewUint(h.GasLimit))
	vv.Set(arena.NewUint(h.GasUsed))
	vv.Set(arena.NewUint(h.Timestamp))

	vv.Set(arena.NewCopyBytes(h.ExtraData))
	vv.Set(arena.NewCopyBytes(h.MixHash.Bytes()))
	vv.Set(arena.NewCopyBytes(h.Nonce[:]))

	vv.Set(arena.NewUint(h.BaseFee))

	return vv
}

func (r Receipts) MarshalRLPTo(dst []byte) []byte {
	return MarshalRLPTo(r.MarshalRLPWith, dst)
}

func (r *Receipts) MarshalRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	for _, rr := range *r {
		if !rr.IsLegacyTx() {
			vv.Set(a.NewCopyBytes([]byte{byte(rr.TransactionType)}))
		}

		vv.Set(rr.MarshalRLPWith(a))
	}

	return vv
}

func (r *Receipt) MarshalRLP() []byte {
	return r.MarshalRLPTo(nil)
}

func (r *Receipt) MarshalRLPTo(dst []byte) []byte {
	if !r.IsLegacyTx() {
		dst = append(dst, byte(r.TransactionType))
	}

	return MarshalRLPTo(r.MarshalRLPWith, dst)
}

// MarshalRLPWith marshals a receipt with a specific fastrlp.Arena
func (r *Receipt) MarshalRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	vv := a.NewArray()

	if r.Status != nil {
		vv.Set(a.NewUint(uint64(*r.Status)))
	} else {
		vv.Set(a.NewCopyBytes(r.Root[:]))
	}

	vv.Set(a.NewUint(r.CumulativeGasUsed))
	vv.Set(a.NewCopyBytes(r.LogsBloom[:]))
	vv.Set(r.MarshalLogsWith(a))

	return vv
}

// MarshalLogsWith marshals the logs of the receipt to RLP with a specific fastrlp.Arena
func (r *Receipt) MarshalLogsWith(a *fastrlp.Arena) *fastrlp.Value {
	if len(r.Logs) == 0 {
		// There are no receipts, write the RLP null array entry
		return a.NewNullArray()
	}

	logs := a.NewArray()

	for _, l := range r.Logs {
		logs.Set(l.MarshalRLPWith(a))
	}

	return logs
}

func (l *Log) MarshalRLPWith(a *fastrlp.Arena) *fastrlp.Value {
	v := a.NewArray()
	v.Set(a.NewCopyBytes(l.Address.Bytes()))

	topics := a.NewArray()
	for _, t := range l.Topics {
		topics.Set(a.NewCopyBytes(t.Bytes()))
	}

	v.Set(topics)
	v.Set(a.NewCopyBytes(l.Data))

	return v
}

func (t *Transaction) MarshalRLP() []byte {
	return t.MarshalRLPTo(nil)
}

func (t *Transaction) MarshalRLPTo(dst []byte) []byte {
	if t.Type != LegacyTx {
		dst = append(dst, byte(t.Type))
	}

	return MarshalRLPTo(t.MarshalRLPWith, dst)
}

// MarshalRLPWith marshals the transaction to RLP with a specific fastrlp.Arena
// Be careful! This function does not serialize tx type as a first byte.
// Use MarshalRLP/MarshalRLPTo in most cases
func (t *Transaction) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	// Check Transaction1559Payload there https://eips.ethereum.org/EIPS/eip-1559#specification
	if t.Type == DynamicFeeTx {
		vv.Set(arena.NewBigInt(t.ChainID))
	}

	vv.Set(arena.NewUint(t.Nonce))

	if t.Type == DynamicFeeTx {
		// Add EIP-1559 related fields.
		// For non-dynamic-fee-tx gas price is used.
		vv.Set(arena.NewBigInt(t.GasTipCap))
		vv.Set(arena.NewBigInt(t.GasFeeCap))
	} else {
		vv.Set(arena.NewBigInt(t.GasPrice))
	}

	vv.Set(arena.NewUint(t.Gas))

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewCopyBytes(t.To.Bytes()))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewBigInt(t.Value))
	vv.Set(arena.NewCopyBytes(t.Input))

	// Specify access list as per spec.
	// This is needed to have the same format as other EVM chains do.
	// There is no access list feature here, so it is always empty just to be compatible.
	// Check Transaction1559Payload there https://eips.ethereum.org/EIPS/eip-1559#specification
	if t.Type == DynamicFeeTx {
		vv.Set(arena.NewArray())
	}

	// signature values
	vv.Set(arena.NewBigInt(t.V))
	vv.Set(arena.NewBigInt(t.R))
	vv.Set(arena.NewBigInt(t.S))

	if t.Type == StateTx {
		vv.Set(arena.NewCopyBytes(t.From.Bytes()))
	}

	return vv
}
