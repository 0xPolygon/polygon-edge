package polybftsecrets

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
)

type Results []command.CommandResult

func (r Results) GetOutput() string {
	var buffer bytes.Buffer

	for _, result := range r {
		buffer.WriteString(result.GetOutput())
	}

	return buffer.String()
}

type SecretsInitResult struct {
	Address       types.Address `json:"address"`
	BLSPubkey     string        `json:"bls_pubkey"`
	NodeID        string        `json:"node_id"`
	PrivateKey    string        `json:"private_key"`
	BLSPrivateKey string        `json:"bls_private_key"`
	Insecure      bool          `json:"insecure"`
	Generated     string        `json:"generated"`
}

func (r *SecretsInitResult) GetOutput() string {
	var buffer bytes.Buffer

	vals := make([]string, 0, 3)

	vals = append(
		vals,
		fmt.Sprintf("Public key (address)|%s", r.Address.String()),
	)

	if r.PrivateKey != "" {
		vals = append(
			vals,
			fmt.Sprintf("Private key|%s", r.PrivateKey),
		)
	}

	if r.BLSPrivateKey != "" {
		vals = append(
			vals,
			fmt.Sprintf("BLS Private key|%s", r.BLSPrivateKey),
		)
	}

	if r.BLSPubkey != "" {
		vals = append(
			vals,
			fmt.Sprintf("BLS Public key|%s", r.BLSPubkey),
		)
	}

	vals = append(vals, fmt.Sprintf("Node ID|%s", r.NodeID))

	if r.Insecure {
		buffer.WriteString("\n[WARNING: INSECURE LOCAL SECRETS - SHOULD NOT BE RUN IN PRODUCTION]\n")
	}

	if r.Generated != "" {
		buffer.WriteString("\n[SECRETS GENERATED]\n")
		buffer.WriteString(r.Generated)
		buffer.WriteString("\n")
	}

	buffer.WriteString("\n[SECRETS INIT]\n")
	buffer.WriteString(helper.FormatKV(vals))
	buffer.WriteString("\n")

	return buffer.String()
}
