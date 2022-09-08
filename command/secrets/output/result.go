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

	// returns the raw output if flag matches
	if r.outputNodeID {
		return r.NodeID
	}

	if r.outputValidator {
		return r.Address
	}

	if r.outputBLS {
		return r.BLSPubkey
	}

	vals := make([]string, 0, 3)

	if r.Address != "" {
		vals = append(
			vals,
			fmt.Sprintf("Public key (address)|%s", r.Address),
		)
	}

	if r.BLSPubkey != "" {
		vals = append(
			vals,
			fmt.Sprintf("BLS Public key|%s", r.BLSPubkey),
		)
	}

	if r.NodeID != "" {
		vals = append(
			vals,
			fmt.Sprintf("Node ID|%s", r.NodeID),
		)
	}

	buffer.WriteString("\n[SECRETS OUTPUT]\n")
	buffer.WriteString(helper.FormatKV(vals))

	buffer.WriteString("\n")

	return buffer.String()
}
