package web3

import "github.com/umbracle/fastrlp"

func (t *Transaction) MarshalRLP() []byte {
	ar := fastrlp.DefaultArenaPool.Get()
	v := t.MarshalRLPWith(ar)
	data := v.MarshalTo(nil)
	fastrlp.DefaultArenaPool.Put(ar)
	return data
}

// MarshalRLPWith marshals the transaction to RLP with a specific fastrlp.Arena
func (t *Transaction) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(t.Nonce))
	vv.Set(arena.NewUint(t.GasPrice))
	vv.Set(arena.NewUint(t.Gas))

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewBytes((*t.To)[:]))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewBigInt(t.Value))
	vv.Set(arena.NewCopyBytes(t.Input))

	// signature values
	vv.Set(arena.NewCopyBytes(t.V))
	vv.Set(arena.NewCopyBytes(t.R))
	vv.Set(arena.NewCopyBytes(t.S))

	return vv
}
