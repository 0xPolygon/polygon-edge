package generate

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/secrets"
)

func GetCommand() *cobra.Command {
	secretsGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Initializes the secrets manager configuration in the provided directory.",
		Run:   runCommand,
	}

	setFlags(secretsGenerateCmd)
	helper.SetRequiredFlags(secretsGenerateCmd, params.getRequiredFlags())

	return secretsGenerateCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dir,
		dirFlag,
		defaultConfigFileName,
		"the directory for the secrets manager configuration file",
	)

	cmd.Flags().StringVar(
		&params.token,
		tokenFlag,
		"",
		"the access token for the service",
	)

	cmd.Flags().StringVar(
		&params.serverURL,
		serverURLFlag,
		"",
		"the server URL for the service",
	)

	cmd.Flags().StringVar(
		&params.serviceType,
		typeFlag,
		string(secrets.HashicorpVault),
		fmt.Sprintf(
			"the type of the secrets manager. Available types: %s, %s and %s",
			secrets.HashicorpVault,
			secrets.AWSSSM,
			secrets.GCPSSM,
		),
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		defaultNodeName,
		"the name of the node for on-service record keeping",
	)

	cmd.Flags().StringVar(
		&params.namespace,
		namespaceFlag,
		defaultNamespace,
		"the namespace for the service",
	)

	cmd.Flags().StringVar(
		&params.extra,
		extraFlag,
		"",
		"Specifies the extra fields map in string format 'key1=val1,key2=val2'",
	)
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.writeSecretsConfig(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
