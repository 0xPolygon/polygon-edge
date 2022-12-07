package bridge

import (
	"context"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/proto"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
)

func GetCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "bridge",
		Short: "",
		Run: func(cmd *cobra.Command, args []string) {
			client, err := helper.GrpcClient(cmd)
			if err != nil {
				panic(err)
			}

			wallet, _, err := genesis.GetSecrets("test-chain-1")
			if err != nil {
				panic(err)
			}

			//fmt.Println(wallet)

			txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress("http://localhost:9545"))
			if err != nil {
				panic(err)
			}

			clt := proto.NewPolybftClient(client)

			/*
				resp1, err := clt.Bridge(context.Background(), &proto.BridgeRequest{})
				if err != nil {
					panic(err)
				}
			*/

			//fmt.Println(resp1)

			index, err := strconv.Atoi(args[0])
			if err != nil {
				panic(err)
			}

			resp2, err := clt.BridgeCall(context.Background(), &proto.BridgeCallRequest{Index: uint64(index)})
			if err != nil {
				panic(err)
			}

			input, err := hex.DecodeString(resp2.Data)
			if err != nil {
				panic(err)
			}

			txn := &ethgo.Transaction{
				To:    (*ethgo.Address)(&contracts.StateReceiverContract),
				Input: input,
			}
			receipt, err := txRelayer.SendTransaction(txn, wallet.Ecdsa)
			if err != nil {
				panic(err)
			}

			//fmt.Println(receipt)
			fmt.Println(receipt.Status)
			//fmt.Println(receipt.Logs)
		},
	}
}
