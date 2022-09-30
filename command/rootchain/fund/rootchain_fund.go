package fund

import "github.com/spf13/cobra"

var (
	params fundParams
)

// GetCommand returns the rootchain fund command
func GetCommand() *cobra.Command {
	rootchainFundCmd := &cobra.Command{
		Use:     "fund",
		Short:   "Fund funds all the genesis addresses",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(rootchainFundCmd)

	return rootchainFundCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.password,
		passwordFlag,
		"",
		"password file to use for non-interactive password input",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {

}
