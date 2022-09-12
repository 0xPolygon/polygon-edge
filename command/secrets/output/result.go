package output

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type SecretsOutputAllResult struct {
	Address   string `json:"address"`
	BLSPubkey string `json:"bls"`
	NodeID    string `json:"node_id"`
}

type SecretsOutputNodeIDResult struct {
	NodeID string `json:"node_id"`
}

type SecretsOutputBLSResult struct {
	BLSPubkey string `json:"bls"`
}

type SecretsOutputValidatorResult struct {
	Address string `json:"address"`
}

func (r *SecretsOutputNodeIDResult) GetOutput() string {
	return r.NodeID
}

func (r *SecretsOutputValidatorResult) GetOutput() string {
	return r.Address
}

func (r *SecretsOutputBLSResult) GetOutput() string {
	return r.BLSPubkey
}

func (r *SecretsOutputAllResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)

	vals = append(
		vals,
		fmt.Sprintf("Public key (address)|%s", r.Address),
	)

	vals = append(
		vals,
		fmt.Sprintf("BLS Public key|%s", r.BLSPubkey),
	)

	vals = append(
		vals,
		fmt.Sprintf("Node ID|%s", r.NodeID),
	)

	buffer.WriteString("\n[SECRETS OUTPUT]\n")
	buffer.WriteString(helper.FormatKV(vals))

	buffer.WriteString("\n")

	return buffer.String()
}
