package fund

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
)

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
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	var pwd string
	if params.password != "" {
		pwdRaw, err := os.ReadFile(params.password)
		if err != nil {
			outputter.SetError(err)

			return
		}
		pwd = string(pwdRaw)
	}

	// Read accounts from genesis
	targets, err := utils.ReadValidatorsByRegexp("", "test-dir", pwd)
	if err != nil {
		outputter.SetError(err)

		return
	}

	for _, target := range targets {
		c.UI.Output(fmt.Sprintf("fund account %s", target.Account.Ecdsa.Address()))
		if err = helper.FundAccount(target.Account.Ecdsa.Address()); err != nil {
			outputter.SetError(err)

			return
		}
	}
}
