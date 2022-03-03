package status

import (
	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	peersStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Returns the status of the specified peer, using the libp2p ID of the peer node",
		Run:   runCommand,
	}

	setFlags(peersStatusCmd)
	setRequiredFlags(peersStatusCmd)

	return peersStatusCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.peerID,
		peerIDFlag,
		"",
		"libp2p node ID of a specific peer within p2p network",
	)
}

func setRequiredFlags(cmd *cobra.Command) {
	for _, requiredFlag := range params.getRequiredFlags() {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	if err := params.initPeerInfo(helper.GetGRPCAddress(cmd)); err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(params.getResult())
}
