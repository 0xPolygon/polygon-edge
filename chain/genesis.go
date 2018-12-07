package chain

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
)

type Genesis struct {
	Nonce      uint64         `json:"nonce"`
	Timestamp  uint64         `json:"timestamp"`
	ExtraData  []byte         `json:"extraData"`
	GasLimit   uint64         `json:"gasLimit"`
	Difficulty *big.Int       `json:"difficulty"`
	Mixhash    common.Hash    `json:"mixHash"`
	Coinbase   common.Address `json:"coinbase"`
	Alloc      GenesisAlloc   `json:"alloc"`

	// Only for testing
	Number     uint64      `json:"number"`
	GasUsed    uint64      `json:"gasUsed"`
	ParentHash common.Hash `json:"parentHash"`
}

type GenesisAlloc struct {
}

// Encoding

func (g *Genesis) UnmarshalJSON(data []byte) error {
	return nil
}

func (g *Genesis) MarshalJSON() ([]byte, error) {
	return nil, nil
}
