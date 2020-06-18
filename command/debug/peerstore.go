package debug

import (
	"fmt"

	"github.com/0xPolygon/minimal/network"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
)

var peersStoreCmd = &cobra.Command{
	Use:   "peerstore",
	Short: "utility tool to debug a boltdb peerstore",
	RunE:  peersInfoRunE,
}

func init() {
	debugCmd.AddCommand(peersStoreCmd)
}

func peersInfoRunE(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one argument")
	}
	peerstore, err := network.NewBoltDBPeerStore(args[0])
	if err != nil {
		return err
	}
	peers, err := peerstore.Load()
	if err != nil {
		return err
	}
	fmt.Println(formatPeers(peers))
	return nil
}

func formatPeers(peers []string) string {
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

func formatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	return columnize.Format(in, columnConf)
}
