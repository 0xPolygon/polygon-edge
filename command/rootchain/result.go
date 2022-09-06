package rootchain

import "bytes"

type RootchainResult struct {
	Message string `json:"message"`
}

func (r *RootchainResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[ROOTCHAIN CONFIG GENERATED]\n")
	buffer.WriteString(r.Message)

	return buffer.String()
}
