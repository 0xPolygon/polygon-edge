package emit

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type result struct {
	Address string   `json:"address"`
	Wallets []string `json:"wallets"`
	Amounts []string `json:"amounts"`
}

func (r *result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Contract (address)|%s", r.Address))
	vals = append(vals, fmt.Sprintf("Wallets|%s", strings.Join(r.Wallets, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))

	buffer.WriteString("\n[ROOTCHAIN EMIT]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
