package status

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type StatusResult struct {
	ChainID            int64  `json:"chain_id"`
	CurrentBlockNumber int64  `json:"current_block_number"`
	CurrentBlockHash   string `json:"current_block_hash"`
	LibP2PAddress      string `json:"libp2p_address"`
}

func (r *StatusResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[CLIENT STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Network (Chain ID)|%d", r.ChainID),
		fmt.Sprintf("Current Block Number (base 10)|%d", r.CurrentBlockNumber),
		fmt.Sprintf("Current Block Hash|%s", r.CurrentBlockHash),
		fmt.Sprintf("Libp2p Address|%s", r.LibP2PAddress),
	}))

	return buffer.String()
}
