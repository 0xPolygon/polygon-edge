package contractdeployment

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/types"
)

type ContractDeploymentResult struct {
	AddAddress    []types.Address `json:"addAddress"`
	RemoveAddress []types.Address `json:"removeAddress"`
	Whitelist     []types.Address `json:"whitelist"`
}

func (r *ContractDeploymentResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[CONTRACT DEPLOYMENT WHITELIST CHANGES]\n\n")

	if len(r.AddAddress) != 0 {
		buffer.WriteString(fmt.Sprintf("Added addresses: %s,\n", r.AddAddress))
	}

	if len(r.RemoveAddress) != 0 {
		buffer.WriteString(fmt.Sprintf("Removed addresses: %s,\n", r.RemoveAddress))
	}

	buffer.WriteString(fmt.Sprintf("Contract deployment whitelist : %s,\n", r.Whitelist))

	return buffer.String()
}
