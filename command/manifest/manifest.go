package manifest

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
)

// GetCommand returns the rootchain emit command
func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Initializes manifest file",
		// PreRunE: runPreRun,
		// Run:     runCommand,
	}

	setFlags(cmd)

	return cmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.manifestPath,
		manifestPathFlag,
		defaultManifestPath,
		"the amount which will be pre-mined to all the validators",
	)
	cmd.Flags().StringArrayVar(
		&params.validators,
		validatorsFlag,
		[]string{},
		"validators defined by user throughout a parameter (format: <node id>:<ECDSA address>:<public BLS key>)",
	)
	cmd.Flags().StringVar(
		&params.premineValidators,
		premineValidatorsFlag,
		command.DefaultPremineBalance,
		"the amount which will be pre-mined to all the validators",
	)
	cmd.Flags().StringVar(
		&params.validatorsPrefixPath,
		validatorsPrefixFlag,
		defaultPolyBftValidatorPrefixPath,
		"prefix path for polybft validator folder directory",
	)

	cmd.MarkFlagsMutuallyExclusive(validatorsFlag, premineValidatorsFlag)
	cmd.MarkFlagsMutuallyExclusive(validatorsFlag, premineValidatorsFlag)
}
