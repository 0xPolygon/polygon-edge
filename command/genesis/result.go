package genesis

import (
	"bytes"
)

type GenesisResult struct {
	Message string `json:"message"`
}

func (r *GenesisResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[GENESIS SUCCESS]\n")
	buffer.WriteString(r.Message)

	return buffer.String()
}
