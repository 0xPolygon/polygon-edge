package polybftgenesis

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

var params = &genesisParams{}

type GenesisResult struct {
	Message string `json:"message"`
}

func (r *GenesisResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[GENESIS SUCCESS]\n")
	buffer.WriteString(r.Message)

	return buffer.String()
}

func GetCommand() *cobra.Command {
	genesisCmd := &cobra.Command{
		Use:     "genesis-polybft",
		Short:   "Generates the genesis configuration file for polybft with the passed in parameters",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(genesisCmd)

	params.setFlags(genesisCmd)

	helper.SetRequiredFlags(genesisCmd, params.getRequiredFlags())

	return genesisCmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	if err := params.validateFlags(); err != nil {
		return err
	}

	return nil
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	config, err := params.GetChainConfig()
	if err != nil {
		outputter.SetError(err)

		return
	}

	if err := helper.WriteGenesisConfigToDisk(config, params.genesisPath); err != nil {
		outputter.SetError(err)

		return
	}

	result := &GenesisResult{
		Message: fmt.Sprintf("Genesis written to %s\n", params.genesisPath),
	}
	outputter.SetCommandResult(result)
}
