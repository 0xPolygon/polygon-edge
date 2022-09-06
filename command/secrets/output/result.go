package output

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type SecretsOutputResult struct {
	Address   string `json:"address,omitempty"`
	BLSPubkey string `json:"bls,omitempty"`
	NodeID    string `json:"node_id,omitempty"`

	outputNodeID    bool `json:"-"`
	outputBLS       bool `json:"-"`
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

	if r.outputBLS {
		buffer.WriteString(r.BLSPubkey)

		return buffer.String()
	}

	vals := make([]string, 0, 3)

	vals = append(
		vals,
		fmt.Sprintf("Public key (address)|%s", r.Address),
	)

	if r.BLSPubkey != "" {
		vals = append(
			vals,
			fmt.Sprintf("BLS Public key|%s", r.BLSPubkey),
		)
	}

	vals = append(vals, fmt.Sprintf("Node ID|%s", r.NodeID))

	buffer.WriteString("\n[SECRETS OUTPUT]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
