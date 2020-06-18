package types

import (
	"fmt"

	"github.com/0xPolygon/minimal/helper/keccak"
	"github.com/umbracle/fastrlp"
)

// UnmarshalRLP unmarshals a Transaction in RLP format
func (t *Transaction) UnmarshalRLP(p *fastrlp.Parser, v *fastrlp.Value) error {
	elems, err := v.GetElems()
	if err != nil {
		return err
	}
	if num := len(elems); num != 9 {
		return fmt.Errorf("not enough elements to decode transaction, expected 9 but found %d", num)
	}

	hash := keccak.DefaultKeccakPool.Get()
	hash.WriteRlp(t.Hash[:0], v)
	keccak.DefaultKeccakPool.Put(hash)

	// nonce
	if t.Nonce, err = elems[0].GetUint64(); err != nil {
		return err
	}
	// gasPrice
	if t.GasPrice, err = elems[1].GetBytes(t.GasPrice[:0]); err != nil {
		return err
	}
	// gas
	if t.Gas, err = elems[2].GetUint64(); err != nil {
		return err
	}
	// to
	vv, err := v.Get(3).Bytes()
	if len(vv) == 20 {
		// address
		addr := BytesToAddress(vv)
		t.To = &addr
	} else {
		// reset To
		t.To = nil
	}
	// value
	if t.Value, err = elems[4].GetBytes(t.Value[:0]); err != nil {
		return err
	}
	// input
	if t.Input, err = elems[5].GetBytes(t.Input[:0]); err != nil {
		return err
	}
	// v
	vv, err = v.Get(6).Bytes()
	if err != nil {
		return err
	}
	if len(vv) != 1 {
		return fmt.Errorf("only one byte expected")
	}
	t.V = byte(vv[0])
	// R
	if t.R, err = elems[7].GetBytes(t.R[:0]); err != nil {
		return err
	}
	// S
	if t.S, err = elems[8].GetBytes(t.S[:0]); err != nil {
		return err
	}
	return nil
}

// MarshalWith marshals the transaction to RLP with a specific fastrlp.Arena
func (t *Transaction) MarshalWith(arena *fastrlp.Arena) *fastrlp.Value {
	vv := arena.NewArray()

	vv.Set(arena.NewUint(t.Nonce))
	vv.Set(arena.NewCopyBytes(t.GasPrice))
	vv.Set(arena.NewUint(t.Gas))

	// Address may be empty
	if t.To != nil {
		vv.Set(arena.NewBytes((*t.To).Bytes()))
	} else {
		vv.Set(arena.NewNull())
	}

	vv.Set(arena.NewCopyBytes(t.Value))
	vv.Set(arena.NewCopyBytes(t.Input))

	// signature values
	vv.Set(arena.NewUint(uint64(t.V)))
	vv.Set(arena.NewCopyBytes(t.R))
	vv.Set(arena.NewCopyBytes(t.S))

	return vv
}
