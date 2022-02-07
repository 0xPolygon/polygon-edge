package generate

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/secrets"
)

func GetCommand() *cobra.Command {
	secretsGenerateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Initializes the secrets manager configuration in the provided directory. Used for Hashicorp Vault",
		Run:   runCommand,
	}

	setFlags(secretsGenerateCmd)
	setRequiredFlags(secretsGenerateCmd)

	return secretsGenerateCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dir,
		dirFlag,
		defaultConfigFileName,
		fmt.Sprintf(
			"the directory for the secrets manager configuration file Default: %s",
			defaultConfigFileName,
		),
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
			"the type of the secrets manager. Default: %s",
			secrets.HashicorpVault,
		),
	)

	cmd.Flags().StringVar(
		&params.name,
		nameFlag,
		defaultNodeName,
		fmt.Sprintf(
			"the name of the node for on-service record keeping. Default: %s",
			defaultNodeName,
		),
	)

	cmd.Flags().StringVar(
		&params.namespace,
		namespaceFlag,
		defaultNamespace,
		fmt.Sprintf(
			"the namespace for the service. Default %s",
			defaultNamespace,
		),
	)
}

func setRequiredFlags(cmd *cobra.Command) {
	for _, requiredFlag := range params.getRequiredFlags() {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := output.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.writeSecretsConfig(); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
