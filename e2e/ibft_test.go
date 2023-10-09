package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	ibftSigner "github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/validators"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/ethgo"
)

// TestIbft_Transfer sends a transfer transaction (EOA -> EOA)
// and verifies it was mined
func TestIbft_Transfer(t *testing.T) {
	const defaultBlockTime uint64 = 2

	testCases := []struct {
		name            string
		blockTime       uint64
		ibftBaseTimeout uint64
		validatorType   validators.ValidatorType
	}{
		{
			name:            "default block time",
			blockTime:       defaultBlockTime,
			ibftBaseTimeout: 0, // use default value
			validatorType:   validators.ECDSAValidatorType,
		},
		{
			name:            "longer block time",
			blockTime:       10,
			ibftBaseTimeout: 20,
			validatorType:   validators.ECDSAValidatorType,
		},
		{
			name:            "with BLS",
			blockTime:       defaultBlockTime,
			ibftBaseTimeout: 0, // use default value
			validatorType:   validators.BLSValidatorType,
		},
	}

	var (
		senderKey, senderAddr = tests.GenerateKeyAndAddr(t)
		_, receiverAddr       = tests.GenerateKeyAndAddr(t)
	)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ibftManager := framework.NewIBFTServersManager(t,
				IBFTMinNodes,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(senderAddr, framework.EthToWei(10))
					config.SetBlockTime(tc.blockTime)
					config.SetIBFTBaseTimeout(tc.ibftBaseTimeout)
					config.SetValidatorType(tc.validatorType)
				},
			)

			var (
				startTimeout = time.Duration(tc.ibftBaseTimeout+60) * time.Second
				txTimeout    = time.Duration(tc.ibftBaseTimeout+20) * time.Second
			)

			ctxForStart, cancelStart := context.WithTimeout(context.Background(), startTimeout)
			defer cancelStart()

			ibftManager.StartServers(ctxForStart)

			txn := &framework.PreparedTransaction{
				From:     senderAddr,
				To:       &receiverAddr,
				GasPrice: ethgo.Gwei(2),
				Gas:      1000000,
				Value:    framework.EthToWei(1),
			}

			ctxForTx, cancelTx := context.WithTimeout(context.Background(), txTimeout)
			defer cancelTx()

			// send tx and wait for receipt
			receipt, err := ibftManager.
				GetServer(0).
				SendRawTx(ctxForTx, txn, senderKey)

			assert.NoError(t, err)
			if receipt == nil {
				t.Fatalf("receipt not received")
			}

			assert.NotNil(t, receipt.TransactionHash)
		})
	}
}

func TestIbft_TransactionFeeRecipient(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
			_, receiverAddr := tests.GenerateKeyAndAddr(t)

			ibftManager := framework.NewIBFTServersManager(
				t,
				IBFTMinNodes,
				IBFTDirPrefix,
				func(i int, config *framework.TestServerConfig) {
					config.Premine(senderAddr, framework.EthToWei(10))
				})

			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			ibftManager.StartServers(ctx)

			srv := ibftManager.GetServer(0)
			clt := srv.JSONRPC()

			txn := &framework.PreparedTransaction{
				From:     senderAddr,
				To:       &receiverAddr,
				GasPrice: ethgo.Gwei(1),
				Gas:      1000000,
				Value:    tc.txAmount,
			}

			if tc.contractCall {
				// Deploy contract
				deployTx := &framework.PreparedTransaction{
					From:     senderAddr,
					GasPrice: ethgo.Gwei(1), // fees should be greater than base fee
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

			if receipt == nil {
				t.Fatalf("receipt not received")
			}

			// Get the block proposer from the extra data seal
			assert.NotNil(t, receipt.BlockHash)
			block, err := clt.Eth().GetBlockByHash(receipt.BlockHash, false)
			assert.NoError(t, err)
			extraData := &ibftSigner.IstanbulExtra{
				Validators:           validators.NewECDSAValidatorSet(),
				CommittedSeals:       &ibftSigner.SerializedSeal{},
				ParentCommittedSeals: &ibftSigner.SerializedSeal{},
			}
			extraDataWithoutVanity := block.ExtraData[ibftSigner.IstanbulExtraVanity:]

			err = extraData.UnmarshalRLP(extraDataWithoutVanity)
			assert.NoError(t, err)

			proposerAddr, err := framework.EcrecoverFromBlockhash(types.Hash(block.Hash), extraData.ProposerSeal)
			assert.NoError(t, err)

			// Given that this is the first transaction on the blockchain, proposer's balance should be equal to the tx fee
			balanceProposer, err := clt.Eth().GetBalance(ethgo.Address(proposerAddr), ethgo.Latest)
			assert.NoError(t, err)

			txFee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), txn.GasPrice)
			assert.Equalf(t, txFee, balanceProposer, "Proposer didn't get appropriate transaction fee")
		})
	}
}
