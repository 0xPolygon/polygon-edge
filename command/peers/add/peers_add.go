package add

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	peersAddCmd := &cobra.Command{
		Use:     "add",
		Short:   "Adds new peers to the peer list, using the peer's libp2p address",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	setFlags(peersAddCmd)
	helper.SetRequiredFlags(peersAddCmd, params.getRequiredFlags())

	return peersAddCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringArrayVar(
		&params.peerAddresses,
		addrFlag,
		[]string{},
		"the libp2p addresses of the peers",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.initSystemClient(helper.GetGRPCAddress(cmd)); err != nil {
		outputter.SetError(err)

		return
	}

	params.addPeers()

	outputter.SetCommandResult(params.getResult())
}
