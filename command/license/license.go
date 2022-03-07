package license

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/licenses"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "license",
		Short: "Returns Polygon Edge license and dependency attributions",
		Args:  cobra.NoArgs,
		Run:   runCommand,
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	bsdLicenses, err := licenses.GetBSDLicenses()
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(
		&LicenseResult{
			BSDLicenses: bsdLicenses,
		},
	)
}
