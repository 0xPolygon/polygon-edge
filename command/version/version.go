package version

import (
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/version"
	"github.com/spf13/cobra"
)

func runVersionCommand(cmd *cobra.Command, _ []string) {
	output := output.InitializeOutputter(cmd)

	output.SetCommandResult(
		&VersionResult{
			Version: version.GetVersion(),
		},
	)

	output.WriteOutput()
}

func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Returns the current Polygon Edge version",
		Args:  cobra.NoArgs,
		Run:   runVersionCommand,
	}
}
