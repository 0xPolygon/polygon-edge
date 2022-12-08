package tui

import (
	"encoding/hex"
	"fmt"
	"math/big"

	rootchainHelper "github.com/0xPolygon/polygon-edge/command/rootchain/helper"

	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
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
	clt, err := jsonrpc.NewClient("http://localhost:8545")
	if err != nil {
		panic(err)
	}

	fmt.Println(getCheckpointNumber(clt))

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
	// NewView()
}

func getCheckpointNumber(clt *jsonrpc.Client) uint64 {
	checkpointManagerAddr := ethgo.Address(rootchainHelper.CheckpointManagerAddress)

	currentCheckpointBlockNumberMethod := abi.MustNewMethod("function currentCheckpointBlockNumber() returns (uint256)")

	input, err := currentCheckpointBlockNumberMethod.Encode([]interface{}{})
	if err != nil {
		panic(err)
	}

	res, err := clt.Eth().Call(&ethgo.CallMsg{
		To:   &checkpointManagerAddr,
		Data: input,
	}, ethgo.Latest)
	if err != nil {
		panic(err)
	}

	output, err := hex.DecodeString(res[2:])
	if err != nil {
		panic(err)
	}

	xx, err := currentCheckpointBlockNumberMethod.Decode(output)
	if err != nil {
		panic(err)
	}

	return xx["0"].(*big.Int).Uint64()
}
