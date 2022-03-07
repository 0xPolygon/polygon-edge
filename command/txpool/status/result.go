package status

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type TxPoolStatusResult struct {
	Transactions uint64 `json:"transactions"`
}

func (r *TxPoolStatusResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[TXPOOL STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Number of transactions in pool:|%d", r.Transactions),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
