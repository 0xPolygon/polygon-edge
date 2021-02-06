package rlputil

import (
	"bytes"

	itrie "github.com/0xPolygon/minimal/state/immutable-trie"
	"github.com/0xPolygon/minimal/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/umbracle/fastrlp"
)

// Handy tools to encode rlp values during tests

func EncodeBodies(b []*types.Body) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range b {
		v.Set(i.MarshalRLPWith(ar))
	}
	return v.MarshalTo(nil)
}

func EncodeReceipts(r [][]*types.Receipt) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range r {
		vv := ar.NewArray()
		for _, j := range i {
			vv.Set(j.MarshalRLPWith(ar))
		}
		v.Set(vv)
	}
	return v.MarshalTo(nil)
}

func EncodeHeaders(h []*types.Header) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range h {
		v.Set(i.MarshalRLPWith(ar))
	}
	return v.MarshalTo(nil)
}

func GetRlp(t *types.Transaction) []byte {
	enc, _ := rlp.EncodeToBytes(t)
	return enc
}

func DeriveSha(txs []*types.Transaction) types.Hash {
	keybuf := new(bytes.Buffer)
	trie := new(itrie.Trie)
	for i := 0; i < len(txs); i++ {
		keybuf.Reset()
		rlp.Encode(keybuf, uint(i))
		trie.TryUpdate(keybuf.Bytes(), GetRlp(txs[i]))
	}
	return trie.Hash()
}
