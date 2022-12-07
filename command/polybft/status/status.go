package status

import (
	"context"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/spf13/cobra"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := helper.GrpcClient(cmd)
			if err != nil {
				panic(err)
			}

			clt := proto.NewPolybftClient(client)

			resp, err := clt.ConsensusSnapshot(context.Background(), &proto.ConsensusSnapshotRequest{})
			if err != nil {
				panic(err)
			}

			rows := make([]string, len(resp.Validators)+1)
			rows[0] = "Address|Balance"
			for i, d := range resp.Validators {
				rows[i+1] = fmt.Sprintf("%s|%d",
					d.Address,
					d.VotingPower,
				)
			}
			fmt.Println(helper.FormatList(rows))
		},
	}
}
