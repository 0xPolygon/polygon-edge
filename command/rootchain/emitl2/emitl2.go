package emitl2

import (
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var stateSyncAbi = abi.MustNewMethod("function syncState(address receiver, bytes data)")

// GetCommand returns the rootchain emit command
func GetCommand() *cobra.Command {
	rootchainEmitL2Cmd := &cobra.Command{
		Use:   "emitl2",
		Short: "Emit an event from the bridge",
		Run: func(cmd *cobra.Command, _ []string) {
			wallet, _, err := genesis.GetSecrets("test-chain-1")
			if err != nil {
				panic(err)
			}

			fmt.Println(wallet)

			txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress("http://localhost:9545"))
			if err != nil {
				panic(err)
			}

			input, err := stateSyncAbi.Encode([]interface{}{ethgo.Address{}, []byte{}})
			if err != nil {
				panic(err)
			}

			txn := &ethgo.Transaction{
				To:    (*ethgo.Address)(&contracts.L2ExitContract),
				Input: input,
			}
			receipt, err := txRelayer.SendTransaction(txn, wallet.Ecdsa)
			if err != nil {
				panic(err)
			}

			fmt.Println(receipt)
			fmt.Println(receipt.Status)
		},
	}

	return rootchainEmitL2Cmd
}
