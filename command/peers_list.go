package command

import (
	"flag"
	"fmt"
)

type PeersListCommand struct {
	Meta
}

func (c *PeersListCommand) Help() string {
	return ""
}

func (c *PeersListCommand) Synopsis() string {
	return ""
}

func (c *PeersListCommand) Run(args []string) int {
	var format bool

	flags := flag.NewFlagSet("peers list", flag.ContinueOnError)
	flags.BoolVar(&format, "format", false, "")
	flags.Usage = func() {}

	if err := flags.Parse(args); err != nil {
		return 1
	}

	client := c.Meta.Client()

	peers, err := client.PeersList()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error retrieving peers list: %s", err))
		return 1
	}

	c.Ui.Output(formatPeers(peers, format))
	return 0
}

func formatPeers(peers []string, format bool) string {
	if len(peers) == 0 {
		return "No peers found"
	}

	rows := make([]string, len(peers)+1)
	rows[0] = "ID"

	for i, d := range peers {
		rowStr := d
		rows[i+1] = rowStr
	}

	return formatList(rows)
}
