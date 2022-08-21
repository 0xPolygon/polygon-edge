package whitelist

import (
	"github.com/0xPolygon/polygon-edge/command/whitelist/contractdeployment"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "whitelist",
		Short: "Top level command for modifying the Polygon Edge whitelists within config file. Only accepts subcommands.",
	}

	registerSubcommands(configCmd)

	return configCmd
}

func registerSubcommands(baseCmd *cobra.Command) {
	baseCmd.AddCommand(
		contractdeployment.GetCommand(),
	)
}
