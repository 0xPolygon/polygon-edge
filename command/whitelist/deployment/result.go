package deployment

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type DeploymentResult struct {
	AddAddress    []types.Address `json:"add_address,omitempty"`
	RemoveAddress []types.Address `json:"remove_address,omitempty"`
	Whitelist     []types.Address `json:"whitelist"`
}

func (r *DeploymentResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[CONTRACT DEPLOYMENT WHITELIST]\n\n")

	if len(r.AddAddress) != 0 {
		buffer.WriteString(fmt.Sprintf("Added addresses: %s,\n", r.AddAddress))
	}

	if len(r.RemoveAddress) != 0 {
		buffer.WriteString(fmt.Sprintf("Removed addresses: %s,\n", r.RemoveAddress))
	}

	buffer.WriteString(fmt.Sprintf("Contract deployment whitelist : %s,\n", r.Whitelist))

	return buffer.String()
}
