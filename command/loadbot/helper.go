package loadbot

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

func displayTxnsInBlocks(buffer *bytes.Buffer, bd *TxnBlockData) {
	if bd.BlocksRequired != 0 {
		buffer.WriteString("\n\n")

		keys := make([]uint64, 0, bd.BlocksRequired)

		for k := range bd.BlockTransactionsMap {
			keys = append(keys, k)
		}

		sort.Slice(keys, func(i, j int) bool {
			return keys[i] < keys[j]
		})

		formattedStrings := make([]string, 0)

		for _, blockNumber := range keys {
			formattedStrings = append(formattedStrings,
				fmt.Sprintf("Block #%d|%d txns (%d gasUsed / %d gasLimit = %d%%)", 
					blockNumber, 
					bd.BlockTransactionsMap[blockNumber],
					bd.GasDataMap.Blocks[blockNumber].GasUsed,
					bd.GasDataMap.Blocks[blockNumber].GasLimit,
					int(float64(bd.GasDataMap.Blocks[blockNumber].GasUsed) / float64(bd.GasDataMap.Blocks[blockNumber].GasLimit) * 100),
				),
			)
		}

		buffer.WriteString(helper.FormatKV(formattedStrings))
	}
}
