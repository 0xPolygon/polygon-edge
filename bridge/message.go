package bridge

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

type Message struct {
	ID          *big.Int
	Transaction *types.Transaction
}

type MessageWithSignatures struct {
	Message
	Signatures [][]byte
}
