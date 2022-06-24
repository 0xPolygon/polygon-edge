package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/command/server/config"
	"github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

/**
	TestIbft_Transfer sends a transfer transaction (EOA -> EOA)
	and verifies it was mined
**/
func TestIbft_Transfer(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		blockTime       uint64
		ibftBaseTimeout uint64
	}{
		{
			name:            "default block time",
			blockTime:       config.DefaultBlockTime,
			ibftBaseTimeout: 0, // use default value
		},
		{
			name:            "longer block time",
			blockTime:       10,
			ibftBaseTimeout: 20,
		},
	}

	var (
		senderKey, senderAddr = tests.GenerateKeyAndAddr(t)
		_, receiverAddr       = tests.GenerateKeyAndAddr(t)
	)

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ibftManager := framework.NewIBFTServersManager(t,
				IBFTMinNodes,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(senderAddr, framework.EthToWei(10))
					config.SetSeal(true)
					config.SetBlockTime(tc.blockTime)
					config.SetIBFTBaseTimeout(tc.ibftBaseTimeout)
				},
			)

			var (
				startTimeout = time.Duration(tc.ibftBaseTimeout+60) * time.Second
				txTimeout    = time.Duration(tc.ibftBaseTimeout+10) * time.Second
			)

			ctxForStart, cancelStart := context.WithTimeout(context.Background(), startTimeout)
			defer cancelStart()

			ibftManager.StartServers(ctxForStart)

			txn := &framework.PreparedTransaction{
				From:     senderAddr,
				To:       &receiverAddr,
				GasPrice: big.NewInt(10000),
				Gas:      1000000,
				Value:    framework.EthToWei(1),
			}

			ctxForTx, cancelTx := context.WithTimeout(context.Background(), txTimeout)
			defer cancelTx()

			//	send tx and wait for receipt
			receipt, err := ibftManager.
				GetServer(0).
				SendRawTx(ctxForTx, txn, senderKey)

			assert.NoError(t, err)
			assert.NotNil(t, receipt)
			assert.NotNil(t, receipt.TransactionHash)
		})
	}
}

func TestIbft_TransactionFeeRecipient(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		contractCall bool
		txAmount     *big.Int
	}{
		{
			name:         "transfer transaction",
			contractCall: false,
			txAmount:     framework.EthToWei(1),
		},
		{
			name:         "contract function execution",
			contractCall: true,
			txAmount:     big.NewInt(0),
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
			_, receiverAddr := tests.GenerateKeyAndAddr(t)

			ibftManager := framework.NewIBFTServersManager(
				t,
				IBFTMinNodes,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(senderAddr, framework.EthToWei(10))
					config.SetSeal(true)
				})

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			ibftManager.StartServers(ctx)

			srv := ibftManager.GetServer(0)
			clt := srv.JSONRPC()

			txn := &framework.PreparedTransaction{
				From:     senderAddr,
				To:       &receiverAddr,
				GasPrice: big.NewInt(10000),
				Gas:      1000000,
				Value:    tc.txAmount,
			}

			if tc.contractCall {
				// Deploy contract
				deployTx := &framework.PreparedTransaction{
					From:     senderAddr,
					GasPrice: big.NewInt(10),
					Gas:      1000000,
					Value:    big.NewInt(0),
					Input:    framework.MethodSig("setA1"),
				}
				ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
				defer cancel()
				receipt, err := srv.SendRawTx(ctx, deployTx, senderKey)
				assert.NoError(t, err)
				assert.NotNil(t, receipt)

				contractAddr := types.Address(receipt.ContractAddress)
				txn.To = &contractAddr
				txn.Input = framework.MethodSig("setA1")
			}

			ctx1, cancel1 := context.WithTimeout(context.Background(), framework.DefaultTimeout)
			defer cancel1()
			receipt, err := srv.SendRawTx(ctx1, txn, senderKey)
			assert.NoError(t, err)
			assert.NotNil(t, receipt)

			// Get the block proposer from the extra data seal
			assert.NotNil(t, receipt.BlockHash)
			block, err := clt.Eth().GetBlockByHash(receipt.BlockHash, false)
			assert.NoError(t, err)
			extraData := &ibft.IstanbulExtra{}
			extraDataWithoutVanity := block.ExtraData[ibft.IstanbulExtraVanity:]
			err = extraData.UnmarshalRLP(extraDataWithoutVanity)
			assert.NoError(t, err)

			proposerAddr, err := framework.EcrecoverFromBlockhash(types.Hash(block.Hash), extraData.Seal)
			assert.NoError(t, err)

			// Given that this is the first transaction on the blockchain, proposer's balance should be equal to the tx fee
			balanceProposer, err := clt.Eth().GetBalance(ethgo.Address(proposerAddr), ethgo.Latest)
			assert.NoError(t, err)

			txFee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), txn.GasPrice)
			assert.Equalf(t, txFee, balanceProposer, "Proposer didn't get appropriate transaction fee")
		})
	}
}
