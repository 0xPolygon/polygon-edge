package debug

import (
	"github.com/spf13/cobra"
)

// DebugCmd is the debug command
var DebugCmd = &cobra.Command{
	Use:   "debug",
	Short: "Debug", // TODO
	RunE:  peersRunE,
}

func peersRunE(cmd *cobra.Command, args []string) error {
	return nil
}
