package regenesis

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

var (
	jsonRPCAddress string
	blockNumber    int64
)

/*
./polygon-edge regenesis getroot --rpc "http://localhost:10002"
*/
func GetRootCMD() *cobra.Command {
	getRootCmd := &cobra.Command{
		Use:   "getroot",
		Short: "returns state root of old chain",
	}
	getRootCmd.Run = func(cmd *cobra.Command, args []string) {
		outputter := command.InitializeOutputter(getRootCmd)
		defer outputter.WriteOutput()

		rpcClient, err := jsonrpc.NewClient(jsonRPCAddress)
		if err != nil {
			outputter.SetError(fmt.Errorf("connect to client error:%w", err))

			return
		}

		block, err := rpcClient.Eth().GetBlockByNumber(ethgo.BlockNumber(blockNumber), false)
		if err != nil {
			outputter.SetError(fmt.Errorf("get block error:%w", err))

			return
		}

		_, err = outputter.Write([]byte(fmt.Sprintf("state root %s for block %d\n", block.StateRoot, block.Number)))
		if err != nil {
			outputter.SetError(fmt.Errorf("get block error:%w", err))

			return
		}
	}

	getRootCmd.Flags().StringVar(
		&jsonRPCAddress,
		"rpc",
		"",
		"the JSON RPC IP address for old chain",
	)
	getRootCmd.Flags().Int64Var(
		&blockNumber,
		"block",
		int64(ethgo.Latest),
		"Block number of trie snapshot",
	)

	return getRootCmd
}
