package print

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

type SecretsPrintResult struct {
	Address types.Address `json:"address"`
	NodeID  string        `json:"node_id"`
}

func (r *SecretsPrintResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[SECRETS PUBLIC DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", r.Address),
		fmt.Sprintf("Node ID|%s", r.NodeID),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
