package add

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type TxPoolAddResult struct {
	Hash     string `json:"hash"`
	From     string `json:"from"`
	To       string `json:"to"`
	Value    string `json:"value"`
	GasPrice string `json:"gas_price"`
	GasLimit uint64 `json:"gas_limit"`
}

func (r *TxPoolAddResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[ADD TRANSACTION]\n")
	buffer.WriteString("Successfully added transaction:\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("HASH|%s", r.Hash),
		fmt.Sprintf("FROM|%s", r.From),
		fmt.Sprintf("TO|%s", r.To),
		fmt.Sprintf("VALUE|%s", r.Value),
		fmt.Sprintf("GAS PRICE|%s", r.GasPrice),
		fmt.Sprintf("GAS LIMIT|%d", r.GasLimit),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
