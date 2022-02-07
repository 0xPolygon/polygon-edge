package version

import (
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/version"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Returns the current Polygon Edge version",
		Args:  cobra.NoArgs,
		Run:   runCommand,
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	outputter.SetCommandResult(
		&VersionResult{
			Version: version.GetVersion(),
		},
	)
}
