package status

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type PeersStatusResult struct {
	ID        string   `json:"id"`
	Protocols []string `json:"protocols"`
	Addresses []string `json:"addresses"`
}

func (r *PeersStatusResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEER STATUS]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("ID|%s", r.ID),
		fmt.Sprintf("Protocols|%s", r.Protocols),
		fmt.Sprintf("Addresses|%s", r.Addresses),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
