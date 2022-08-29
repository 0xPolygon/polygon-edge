package print

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	secretsPrintCmd := &cobra.Command{
		Use:     "print",
		Short:   "Prints validator key address and public network key from the provided Secrets Manager",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(secretsPrintCmd)

	return secretsPrintCmd
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

	cmd.Flags().BoolVar(
		&params.printNodeID,
		nodeIDFlag,
		false,
		"output only the node id "+
			"from the provided secrets manager",
	)

	cmd.Flags().BoolVar(
		&params.printValidator,
		validatorFlag,
		false,
		"output only the validator key address "+
			"from the provided secrets manager",
	)

	cmd.MarkFlagsMutuallyExclusive(dataDirFlag, configFlag)
	cmd.MarkFlagsMutuallyExclusive(nodeIDFlag, validatorFlag)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.printSecrets(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
