package status

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type TxPoolStatusResult struct {
	Txs uint64 `json:"txs"`
}

func (r *TxPoolStatusResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[TXPOOL STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Number of transactions in pool:|%d", r.Txs),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
