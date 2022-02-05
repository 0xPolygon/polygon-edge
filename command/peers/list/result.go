package list

import (
	"bytes"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
)

type PeersListResult struct {
	Peers []string `json:"peers"`
}

func (r *PeersListResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEERS LIST]\n")

	if len(r.Peers) == 0 {
		buffer.WriteString("No peers found")
	} else {
		buffer.WriteString(fmt.Sprintf("Number of peers: %d\n\n", len(r.Peers)))

		rows := make([]string, len(r.Peers))
		for i, p := range r.Peers {
			rows[i] = fmt.Sprintf("[%d]|%s", i, p)
		}
		buffer.WriteString(helper.FormatKV(rows))
	}

	buffer.WriteString("\n")

	return buffer.String()
}
