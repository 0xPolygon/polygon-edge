package init

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/helper/hex"
)

type SecretsInitResult struct {
	Address []byte `json:"address"`
	NodeID  string `json:"node_id"`
}

func (r *SecretsInitResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[SECRETS INIT]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Public key (address)|%s", hex.EncodeToHex(r.Address)),
		fmt.Sprintf("Node ID|%s", r.NodeID),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
