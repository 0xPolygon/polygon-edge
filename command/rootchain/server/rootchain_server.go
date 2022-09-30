package server

import "github.com/spf13/cobra"

var (
	params serverParams
)

// GetCommand returns the rootchain server command
func GetCommand() *cobra.Command {
	rootchainServerCmd := &cobra.Command{
		Use:     "server",
		Short:   "Start the rootchain command",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainServerCmd)

	return rootchainServerCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.dataDir,
		dataDirFlag,
		"test-rootchain",
		"target directory for the chain",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {

}
