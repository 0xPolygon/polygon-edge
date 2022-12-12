package demo

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/command/genesis"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
)

var claimValidatorReward = abi.MustNewMethod("function claimValidatorReward()")
var validatorRewardClaimed = abi.MustNewEvent("event ValidatorRewardClaimed(address indexed validator, uint256 amount)")
var stakeFunction = abi.MustNewMethod("function stake()")

var stakeEvent = abi.MustNewEvent("event Staked(address indexed validator, uint256 amount)")

// GetCommand returns the rootchain emit command
func StakeCommand() *cobra.Command {
	stakeCmd := &cobra.Command{
		Use:   "stake",
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

			// send the same amount back again
			input2, err := stakeFunction.Encode([]interface{}{})
			if err != nil {
				panic(err)
			}

			txn2 := &ethgo.Transaction{
				To:    (*ethgo.Address)(&contracts.ValidatorSetContract),
				Input: input2,
				Value: big.NewInt(100000),
			}
			receipt2, err := txRelayer.SendTransaction(txn2, wallet.Ecdsa)
			if err != nil {
				panic(err)
			}

			for _, log := range receipt2.Logs {
				if stakeEvent.Match(log) {
					data, err := stakeEvent.ParseLog(log)
					if err != nil {
						panic(err)
					}
					fmt.Println(data)
				}
			}

			fmt.Println(receipt2.Status)
			fmt.Println(receipt2.Logs)

			//xxx := abi.MustNewMethod("function totalActiveStake() public view returns (uint256 activeStake)")

			//fmt.Println(call(client, xxx, ethgo.Address(contracts.ValidatorSetContract), []interface{}{})
		},
	}

	return stakeCmd
}

func call(clt *jsonrpc.Client, m *abi.Method, addr ethgo.Address, args []interface{}) map[string]interface{} {
	input, err := m.Encode(args)
	if err != nil {
		panic(err)
	}

	res, err := clt.Eth().Call(&ethgo.CallMsg{
		To:   &addr,
		Data: input,
	}, ethgo.Latest)
	if err != nil {
		panic(err)
	}

	output, err := hex.DecodeString(res[2:])
	if err != nil {
		panic(err)
	}

	xx, err := m.Decode(output)
	if err != nil {
		panic(err)
	}
	return xx
}
