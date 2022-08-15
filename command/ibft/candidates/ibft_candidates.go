package candidates

import (
	"context"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/command/helper"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/spf13/cobra"
	empty "google.golang.org/protobuf/types/known/emptypb"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "candidates",
		Short: "Queries the current set of proposed candidates, as well as candidates that have not been included yet",
		Run:   runCommand,
	}
}

func runCommand(cmd *cobra.Command, _ []string) {
	outputter := command.InitializeOutputter(cmd)
	defer outputter.WriteOutput()

	candidatesResponse, err := getIBFTCandidates(helper.GetGRPCAddress(cmd))
	if err != nil {
		outputter.SetError(err)

		return
	}

	outputter.SetCommandResult(
		newIBFTCandidatesResult(candidatesResponse),
	)
}

func getIBFTCandidates(grpcAddress string) (*ibftOp.CandidatesResp, error) {
	client, err := helper.GetIBFTOperatorClientConnection(
		grpcAddress,
	)
	if err != nil {
		return nil, err
	}

	return client.Candidates(context.Background(), &empty.Empty{})
}
