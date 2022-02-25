package bridge

import (
	"github.com/0xPolygon/polygon-edge/types"
	"math/big"
)

type Message struct {
	ID          *big.Int
	Transaction *types.Transaction
}

type MessageWithSignatures struct {
	Message
	Signatures [][]byte
}

//func (m *Message) ComputeHash() types.Hash {
//	var hash []byte
//
//	hashObj := keccak.DefaultKeccakPool.Get()
//	// TODO: handle error
//	hashObj.Write(m.MarshalRLP())
//	hashObj.Sum(hash)
//	keccak.DefaultKeccakPool.Put(hashObj)
//
//	return types.BytesToHash(hash)
//}
//
//func (m *Message) MarshalRLP() []byte {
//	return m.MarshalRLPTo(nil)
//}
//
//func (m *Message) MarshalRLPTo(dst []byte) []byte {
//	return types.MarshalRLPTo(m.MarshalRLPWith, dst)
//}
//
//func (m *Message) MarshalRLPWith(arena *fastrlp.Arena) *fastrlp.Value {
//	vv := arena.NewArray()
//
//	vv.Set(arena.NewBytes(m.ID.Bytes()))
//	vv.Set(arena.NewBytes(m.Transaction.MarshalRLP()))
//
//	return vv
//}
