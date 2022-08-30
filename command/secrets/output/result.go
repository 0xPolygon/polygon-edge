package output

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type SecretsOutputResult struct {
	Address string `json:"address,omitempty"`
	NodeID  string `json:"node_id,omitempty"`

	outputNodeID    bool `json:"-"`
	outputValidator bool `json:"-"`
}

func (r *SecretsOutputResult) GetOutput() string {
	var buffer bytes.Buffer

	if r.outputNodeID {
		buffer.WriteString(r.NodeID)
		return buffer.String()
	}
	if r.outputValidator {
		buffer.WriteString(r.Address)
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
