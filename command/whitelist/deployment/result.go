package deployment

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type DeploymentResult struct {
	AddAddresses    []types.Address `json:"addAddress,omitempty"`
	RemoveAddresses []types.Address `json:"removeAddress,omitempty"`
	Whitelist       []types.Address `json:"whitelist"`
}

func (r *DeploymentResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[CONTRACT DEPLOYMENT WHITELIST]\n\n")

	if len(r.AddAddresses) != 0 {
		buffer.WriteString(fmt.Sprintf("Added addresses: %s,\n", r.AddAddresses))
	}

	if len(r.RemoveAddresses) != 0 {
		buffer.WriteString(fmt.Sprintf("Removed addresses: %s,\n", r.RemoveAddresses))
	}

	buffer.WriteString(fmt.Sprintf("Contract deployment whitelist : %s,\n", r.Whitelist))

	return buffer.String()
}
