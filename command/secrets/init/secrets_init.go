package init

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/spf13/cobra"
)

var (
	errInvalidConfig   = errors.New("invalid secrets configuration")
	errInvalidParams   = errors.New("no config file or data directory passed in")
	errUnsupportedType = errors.New("unsupported secrets manager")
)

func GetCommand() *cobra.Command {
	secretsInitCmd := &cobra.Command{
		Use: "init",
		Short: "Initializes private keys for the Polygon Edge (Validator + Networking) " +
			"to the specified Secrets Manager",
		Run: runCommand,
	}

	setFlags(secretsInitCmd)

	return secretsInitCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		"data-dir",
		"",
		"the directory for the Polygon Edge data if the local FS is used",
	)

	cmd.Flags().StringVar(
		&params.configPath,
		"config",
		"",
		"sets the path to the SecretsManager config file, "+
			"if omitted, the local FS secrets manager is used",
	)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if !params.areValidParams() {
		outputter.SetError(errInvalidParams)

		return
	}

	if err := params.initSecrets(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
