package print

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type SecretsPrintResult struct {
	Address string `json:"address,omitempty"`
	NodeID  string `json:"node_id,omitempty"`

	printNodeID    bool `json:"-"`
	printValidator bool `json:"-"`
}

func (r *SecretsPrintResult) GetOutput() string {
	var buffer bytes.Buffer

	if r.printNodeID {
		buffer.WriteString(fmt.Sprintf("%s", r.NodeID))
		return buffer.String()
	}
	if r.printValidator {
		buffer.WriteString(fmt.Sprintf("%s", r.Address))
		return buffer.String()
	}

	buffer.WriteString("\n[SECRETS PUBLIC DATA]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", r.Address),
		fmt.Sprintf("Node ID|%s", r.NodeID),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
