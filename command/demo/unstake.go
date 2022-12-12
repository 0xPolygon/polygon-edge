package demo

import (
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

// GetCommand returns the rootchain emit command
func UnstakeCommand() *cobra.Command {
	stakeCmd := &cobra.Command{
		Use:   "unstake",
		Short: "Emit an event from the bridge",
		Run: func(cmd *cobra.Command, _ []string) {

			wallet, _, err := genesis.GetSecrets("test-chain-3")
			if err != nil {
				panic(err)
			}

			client, err := jsonrpc.NewClient("http://localhost:9545")
			if err != nil {
				panic(err)
			}

			txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(client))
			if err != nil {
				panic(err)
			}

			input, err := claimValidatorReward.Encode([]interface{}{})
			if err != nil {
				panic(err)
			}

			txn := &ethgo.Transaction{
				To:    (*ethgo.Address)(&contracts.ValidatorSetContract),
				Input: input,
			}
			receipt, err := txRelayer.SendTransaction(txn, wallet.Ecdsa)
			if err != nil {
				panic(err)
			}

			var amount *big.Int
			for _, log := range receipt.Logs {
				if validatorRewardClaimed.Match(log) {
					data, err := validatorRewardClaimed.ParseLog(log)
					if err != nil {
						panic(err)
					}
					amount = data["amount"].(*big.Int)
				}
			}

			fmt.Println(amount)
		},
	}

	return stakeCmd
}
