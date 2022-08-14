package add

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type PeersAddResult struct {
	NumRequested int      `json:"num_requested"`
	NumAdded     int      `json:"num_added"`
	Peers        []string `json:"peers"`
	Errors       []string `json:"errors"`
}

func (r *PeersAddResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[PEERS ADDED]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Peers listed|%d", r.NumRequested), // The number of peers the user wanted to add
		fmt.Sprintf("Peers added|%d", r.NumAdded),      // The number of peers that have been added
	}))

	if len(r.Peers) > 0 {
		buffer.WriteString("\n\n[LIST OF ADDED PEERS]\n")
		buffer.WriteString(helper.FormatList(r.Peers))
	}

	if len(r.Errors) > 0 {
		buffer.WriteString("\n\n[ERRORS]\n")
		buffer.WriteString(helper.FormatList(r.Errors))
	}

	buffer.WriteString("\n")

	return buffer.String()
}
