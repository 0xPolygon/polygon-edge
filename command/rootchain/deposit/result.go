package deposit

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type result struct {
	TokenType string   `json:"tokenType"`
	Receivers []string `json:"receivers"`
	Amounts   []string `json:"amounts"`
}

func (r *result) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)
	vals = append(vals, fmt.Sprintf("Token Type|%s", r.TokenType))
	vals = append(vals, fmt.Sprintf("Receivers|%s", strings.Join(r.Receivers, ", ")))
	vals = append(vals, fmt.Sprintf("Amounts|%s", strings.Join(r.Amounts, ", ")))

	buffer.WriteString("\n[DEPOSIT]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
