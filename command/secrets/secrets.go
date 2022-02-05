package secrets

import (
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/command/secrets/init"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	secretsCmd := &cobra.Command{
		Use:   "secrets",
		Short: "Top level SecretsManager command for interacting with secrets functionality. Only accepts subcommands.",
	}

	// Register the base GRPC address flag
	secretsCmd.PersistentFlags().String(
		helper.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", "127.0.0.1", server.DefaultGRPCPort),
		helper.GRPCAddressFlag,
	)

	registerSubcommands(secretsCmd)

	return secretsCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	// secrets init
	baseCmd.AddCommand(init.GetCommand())
}
