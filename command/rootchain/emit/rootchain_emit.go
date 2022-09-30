package emit

import "github.com/spf13/cobra"

// GetCommand returns the rootchain emit command
func GetCommand() *cobra.Command {
	rootchainEmitCmd := &cobra.Command{
		Use:   "emit",
		Short: "",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
		Run: func(cmd *cobra.Command, args []string) {

		},
	}

	return rootchainEmitCmd
}
