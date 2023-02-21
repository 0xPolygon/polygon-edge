package polybftsecrets

import (
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
)

var basicParams = &initParams{}

func GetCommand() *cobra.Command {
	secretsInitCmd := &cobra.Command{
		Use: "polybft-secrets",
		Short: "Initializes private keys for the Polygon Edge (Validator + Networking) " +
			"to the specified Secrets Manager",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	basicParams.setFlags(secretsInitCmd)

	return secretsInitCmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return basicParams.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	results, err := basicParams.Execute()
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(results)
}
