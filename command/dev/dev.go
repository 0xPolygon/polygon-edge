package dev

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	devCmd := &cobra.Command{
		Use: "dev",
		Short: "\"Bypasses\" consensus and networking and starts a blockchain locally. " +
			"It starts a local node and mines every transaction in a separate block",
		RunE: runCommand,
	}

	params.initChildCommands()

	setFlags(devCmd)
	setDevValues()

	return devCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().AddFlagSet(params.genesisCmd.Flags())
	cmd.Flags().AddFlagSet(params.serverCmd.Flags())
}

func setDevValues() {
	// Dev mode always uses the dev consensus
	_ = params.genesisCmd.Flags().Set(command.ConsensusFlag, "dev")

	// Dev mode has disabled networking
	_ = params.serverCmd.Flags().Set(command.NoDiscoverFlag, "true")
	_ = params.serverCmd.Flags().Set(command.BootnodeFlag, dummyBootnode)

	// Dev mode has the DEBUG log level by default
	_ = params.serverCmd.Flags().Set(command.LogLevelFlag, "DEBUG")
}

func runCommand(cmd *cobra.Command, args []string) error {
	if err := params.genesisCmd.Execute(); err != nil {
		return err
	}

	return params.serverCmd.Execute()
}
