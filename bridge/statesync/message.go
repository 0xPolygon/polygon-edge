package statesync

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

type Message struct {
	ID          *big.Int
	Transaction *types.Transaction
}

func (m *Message) Hash() types.Hash {
	return m.Transaction.ComputeHash().Hash
}

type MessageWithSignatures struct {
	Message
	Signatures [][]byte
}
