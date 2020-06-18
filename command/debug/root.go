package debug

import (
	"github.com/0xPolygon/minimal/command"
	"github.com/spf13/cobra"
)

var debugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug", // TODO
	RunE:  peersRunE,
}

func init() {
	command.RegisterCmd(debugCmd)
}

func versionRun(cmd *cobra.Command, args []string) {
	command.RunCmd(cmd, args, peersRunE)
}

func peersRunE(cmd *cobra.Command, args []string) error {
	return nil
}
