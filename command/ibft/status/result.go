package status

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type IBFTStatusResult struct {
	ValidatorKey string `json:"validator_key"`
}

func (r *IBFTStatusResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[VALIDATOR STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Validator key|%s", r.ValidatorKey),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
