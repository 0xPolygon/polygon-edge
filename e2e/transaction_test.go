package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func TestSignedTransaction(t *testing.T) {
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	preminedAmount := framework.EthToWei(10)
	ibftManager := framework.NewIBFTServersManager(t, IBFTMinNodes, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.Premine(senderAddr, preminedAmount)
		config.PremineValidatorBalance(big.NewInt(0), framework.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	clt := srv.JSONRPC()

	// check there is enough balance
	balance, err := clt.Eth().GetBalance(web3.Address(senderAddr), web3.Latest)
	assert.NoError(t, err)
	assert.Equal(t, preminedAmount, balance)

	for i := 0; i < 5; i++ {
		txn := &framework.PreparedTransaction{
			From:     senderAddr,
			To:       &receiverAddr,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    big.NewInt(10000),
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		receipt, err := srv.SendRawTx(ctx, txn, senderKey)
		assert.NoError(t, err)
		assert.NotNil(t, receipt)
		assert.NotNil(t, receipt.TransactionHash)
	}
}

func TestPreminedBalance(t *testing.T) {
	preminedAccounts := []struct {
		address types.Address
		balance *big.Int
	}{
		{types.StringToAddress("1"), big.NewInt(0)},
		{types.StringToAddress("2"), big.NewInt(20)},
	}

	testTable := []struct {
		name    string
		address types.Address
		balance *big.Int
	}{
		{
			"Account with 0 balance",
			preminedAccounts[0].address,
			preminedAccounts[0].balance,
		},
		{
			"Account with valid balance",
			preminedAccounts[1].address,
			preminedAccounts[1].balance,
		},
		{
			"Account not in genesis",
			types.StringToAddress("3"),
			big.NewInt(0),
		},
	}

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		for _, acc := range preminedAccounts {
			config.Premine(acc.address, acc.balance)
		}
	})
	srv := srvs[0]

	rpcClient := srv.JSONRPC()
	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			balance, err := rpcClient.Eth().GetBalance(web3.Address(testCase.address), web3.Latest)
			assert.NoError(t, err)
			assert.Equal(t, testCase.balance, balance)
		})
	}
}

func TestEthTransfer(t *testing.T) {
	validAccounts := []struct {
		address types.Address
		balance *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("1"),
			framework.EthToWei(50), // 50 ETH
		},
		// Empty account
		{
			types.StringToAddress("2"),
			big.NewInt(0),
		},
		// Valid account #2
		{
			types.StringToAddress("3"),
			framework.EthToWei(10), // 10 ETH
		},
	}

	testTable := []struct {
		name          string
		sender        types.Address
		recipient     types.Address
		amount        *big.Int
		shouldSucceed bool
	}{
		{
			// ACC #1 -> ACC #3
			"Valid ETH transfer #1",
			validAccounts[0].address,
			validAccounts[2].address,
			framework.EthToWei(10),
			true,
		},
		{
			// ACC #2 -> ACC #3
			"Invalid ETH transfer",
			validAccounts[1].address,
			validAccounts[2].address,
			framework.EthToWei(100),
			false,
		},
		{
			// ACC #3 -> ACC #2
			"Valid ETH transfer #2",
			validAccounts[2].address,
			validAccounts[1].address,
			framework.EthToWei(5),
			true,
		},
	}

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		for _, acc := range validAccounts {
			config.Premine(acc.address, acc.balance)
		}
	})
	srv := srvs[0]

	rpcClient := srv.JSONRPC()
	for indx, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			// TODO @Vuksan Please remove this skip statement when JSON-RPC Error wrapping is added
			if indx == 1 {
				t.Skip()
			}

			// Fetch the balances before sending
			balanceSender, err := rpcClient.Eth().GetBalance(
				web3.Address(testCase.sender),
				web3.Latest,
			)
			assert.NoError(t, err)

			balanceReceiver, err := rpcClient.Eth().GetBalance(
				web3.Address(testCase.recipient),
				web3.Latest,
			)
			assert.NoError(t, err)

			// Set the preSend balances
			previousSenderBalance := balanceSender
			previousReceiverBalance := balanceReceiver

			// Create the transaction
			toAddr := web3.Address(testCase.recipient)
			txnObject := &web3.Transaction{
				From:     web3.Address(testCase.sender),
				To:       &toAddr,
				GasPrice: uint64(1048576),
				Gas:      1000000,
				Value:    testCase.amount,
			}

			fee := big.NewInt(0)

			// Do the transfer
			txnHash, err := rpcClient.Eth().SendTransaction(txnObject)
			assert.NoError(t, err)
			assert.IsTypef(t, web3.Hash{}, txnHash, "Return type mismatch")

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			receipt, err := srv.WaitForReceipt(ctx, txnHash)

			if testCase.shouldSucceed {
				assert.NoError(t, err)
				assert.NotNil(t, receipt)
			} else { // When an invalid transaction is supplied, there should be no receipt.
				assert.Error(t, err)
				assert.Nil(t, receipt)
			}

			// Fetch the balances after sending
			balanceSender, err = rpcClient.Eth().GetBalance(
				web3.Address(testCase.sender),
				web3.Latest,
			)
			assert.NoError(t, err)

			balanceReceiver, err = rpcClient.Eth().GetBalance(
				web3.Address(testCase.recipient),
				web3.Latest,
			)
			assert.NoError(t, err)

			expectedSenderBalance := previousSenderBalance
			if testCase.shouldSucceed {
				fee = new(big.Int).Mul(
					big.NewInt(int64(receipt.GasUsed)),
					big.NewInt(int64(txnObject.GasPrice)),
				)

				expectedSenderBalance = previousSenderBalance.Sub(
					previousSenderBalance,
					new(big.Int).Add(testCase.amount, fee),
				)
			}
			expectedReceiverBalance := previousReceiverBalance
			if testCase.shouldSucceed {
				expectedReceiverBalance = previousReceiverBalance.Add(
					previousReceiverBalance,
					testCase.amount,
				)
			}

			// Check the balances
			assert.Equalf(t,
				expectedSenderBalance,
				balanceSender,
				"Sender balance incorrect")
			assert.Equalf(t,
				expectedReceiverBalance,
				balanceReceiver,
				"Receiver balance incorrect")
		})
	}
}
