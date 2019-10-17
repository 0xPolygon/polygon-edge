package rlputil

import (
	"github.com/umbracle/fastrlp"
	"github.com/umbracle/minimal/types"
)

// Handy tools to encode rlp values during tests

func EncodeBodies(b []*types.Body) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range b {
		v.Set(i.MarshalWith(ar))
	}
	return v.MarshalTo(nil)
}

func EncodeReceipts(r [][]*types.Receipt) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range r {
		vv := ar.NewArray()
		for _, j := range i {
			vv.Set(j.MarshalWith(ar))
		}
		v.Set(vv)
	}
	return v.MarshalTo(nil)
}

func EncodeHeaders(h []*types.Header) []byte {
	ar := &fastrlp.Arena{}
	v := ar.NewArray()
	for _, i := range h {
		v.Set(i.MarshalWith(ar))
	}
	return v.MarshalTo(nil)
}
