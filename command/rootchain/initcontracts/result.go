package initcontracts

import (
	"bytes"
)

type result struct {
	AlreadyDeployed bool `json:"alreadyDeployed"`
}

func (r *result) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[ROOTCHAIN INIT CONTRACTS]\n")

	if r.AlreadyDeployed {
		buffer.WriteString("Rootchain contracts are already deployed")
	} else {
		buffer.WriteString("Rootchain contracts has been deployed")
	}

	buffer.WriteString("\n")

	return buffer.String()
}
