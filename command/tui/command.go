package tui

import (
	"encoding/hex"
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
	/*
		clt, err := jsonrpc.NewClient("http://localhost:8545")
		if err != nil {
			panic(err)
		}

		fmt.Println(getCheckpointNumber(clt))
		for i := uint64(0); i < getCurrentEpoch(clt); i++ {
			fmt.Println(getCheckpointByNumber(clt, i))
		}
	*/

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

type checkpoint struct {
	Epoch       uint64
	BlockNumber uint64
	EventRoot   [32]byte
}

func getCheckpointByNumber(clt *jsonrpc.Client, num uint64) *checkpoint {
	checkpointManagerAddr := ethgo.Address(rootchainHelper.CheckpointManagerAddress)

	checkpointQuery := abi.MustNewMethod("function checkpoints(uint256) returns (uint256,uint256,bytes32)")

	input, err := checkpointQuery.Encode([]interface{}{new(big.Int).SetUint64(num)})
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

	xx, err := checkpointQuery.Decode(output)
	if err != nil {
		panic(err)
	}

	return &checkpoint{Epoch: xx["0"].(*big.Int).Uint64(), BlockNumber: xx["1"].(*big.Int).Uint64(), EventRoot: xx["2"].([32]byte)}
}

func getCurrentEpoch(clt *jsonrpc.Client) uint64 {
	checkpointManagerAddr := ethgo.Address(rootchainHelper.CheckpointManagerAddress)

	currentCheckpointIDMethod := abi.MustNewMethod("function currentEpoch() returns (uint256)")

	input, err := currentCheckpointIDMethod.Encode([]interface{}{})
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

	xx, err := currentCheckpointIDMethod.Decode(output)
	if err != nil {
		panic(err)
	}

	return xx["0"].(*big.Int).Uint64()
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
