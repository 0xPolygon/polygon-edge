package tui

import (
	"github.com/spf13/cobra"
)

// GetCommand returns the tui command
func GetCommand() *cobra.Command {
	tuiCmd := &cobra.Command{
		Use:   "tui",
		Short: "Tui starts the Terminal UI",
		Run:   runCommand,
	}

	return tuiCmd
}

func runCommand(cmd *cobra.Command, args []string) {
	/*
		client, err := grpc.Dial("localhost:5001", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			panic(err)
		}

		clt := proto.NewPolybftClient(client)

		resp, err := clt.ConsensusSnapshot(context.Background(), &proto.ConsensusSnapshotRequest{})
		if err != nil {
			panic(err)
		}

		fmt.Println(resp.BlockNumber)

		clt.Bridge(context.Background(), &proto.BridgeRequest{})
	*/
	NewView()
}
