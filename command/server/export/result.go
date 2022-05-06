package export

import "bytes"

type cmdResult struct {
	CommandOutput string `json:"export_result"`
}

func (c *cmdResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[EXPORT SUCCESS]\n")
	buffer.WriteString(c.CommandOutput + "\n")

	return buffer.String()
}
