package peers

import (
	"github.com/0xPolygon/minimal/command"
	"github.com/spf13/cobra"
)

var peersCmd = &cobra.Command{
	Use:   "peers",    // TODO: change to a compiler input string?
	Short: "Peers...", // TODO
	Run:   peersRun,
	RunE:  peersRunE,
}

func init() {
	command.RegisterCmd(peersCmd)
}

func peersRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, peersRunE)
}

func peersRunE(cmd *cobra.Command, args []string) error {
	// TODO:
	return nil
}
