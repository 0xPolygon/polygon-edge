package init

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	secretsInitCmd := &cobra.Command{
		Use: "init",
		Short: "Initializes private keys for the Polygon Edge (Validator + Networking) " +
			"to the specified Secrets Manager",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(secretsInitCmd)

	return secretsInitCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		dataDirFlag,
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&params.configPath,
		configFlag,
		"",
		"the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)

	cmd.MarkFlagsMutuallyExclusive(dataDirFlag, configFlag)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.initSecrets(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
