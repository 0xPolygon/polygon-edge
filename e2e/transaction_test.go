package e2e

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
)

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
		config.SetBurnContract(types.ZeroAddress)
		for _, acc := range preminedAccounts {
			config.Premine(acc.address, acc.balance)
		}
	})
	srv := srvs[0]

	rpcClient := srv.JSONRPC()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			balance, err := rpcClient.Eth().GetBalance(ethgo.Address(testCase.address), ethgo.Latest)
			require.NoError(t, err)
			require.Equal(t, testCase.balance, balance)
		})
	}
}

func TestEthTransfer(t *testing.T) {
	accountBalances := []*big.Int{
		framework.EthToWei(50), // 50 ETH
		big.NewInt(0),
		framework.EthToWei(10), // 10 ETH
	}

	validAccounts := make([]testAccount, len(accountBalances))

	for indx := 0; indx < len(accountBalances); indx++ {
		key, addr := tests.GenerateKeyAndAddr(t)

		validAccounts[indx] = testAccount{
			address: addr,
			key:     key,
			balance: accountBalances[indx],
		}
	}

	testTable := []struct {
		name          string
		sender        types.Address
		senderKey     *ecdsa.PrivateKey
		recipient     types.Address
		amount        *big.Int
		shouldSucceed bool
	}{
		{
			// ACC #1 -> ACC #3
			"Valid ETH transfer #1",
			validAccounts[0].address,
			validAccounts[0].key,
			validAccounts[2].address,
			framework.EthToWei(10),
			true,
		},
		{
			// ACC #2 -> ACC #3
			"Invalid ETH transfer",
			validAccounts[1].address,
			validAccounts[1].key,
			validAccounts[2].address,
			framework.EthToWei(100),
			false,
		},
		{
			// ACC #3 -> ACC #2
			"Valid ETH transfer #2",
			validAccounts[2].address,
			validAccounts[2].key,
			validAccounts[1].address,
			framework.EthToWei(5),
			true,
		},
	}

	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		for _, acc := range validAccounts {
			config.Premine(acc.address, acc.balance)
			config.SetBurnContract(types.StringToAddress("0xBurnContract"))
		}
	})
	srv := srvs[0]

	rpcClient := srv.JSONRPC()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			// Fetch the balances before sending
			balanceSender, err := rpcClient.Eth().GetBalance(
				ethgo.Address(testCase.sender),
				ethgo.Latest,
			)
			require.NoError(t, err)

			balanceReceiver, err := rpcClient.Eth().GetBalance(
				ethgo.Address(testCase.recipient),
				ethgo.Latest,
			)
			require.NoError(t, err)

			// Set the preSend balances
			previousSenderBalance := balanceSender
			previousReceiverBalance := balanceReceiver

			// Do the transfer
			ctx, cancel := context.WithTimeout(context.Background(), framework.DefaultTimeout)
			defer cancel()

			txn := &framework.PreparedTransaction{
				From:     testCase.sender,
				To:       &testCase.recipient,
				GasPrice: ethgo.Gwei(1),
				Gas:      1000000,
				Value:    testCase.amount,
			}

			receipt, err := srv.SendRawTx(ctx, txn, testCase.senderKey)

			if testCase.shouldSucceed {
				require.NoError(t, err)
				require.NotNil(t, receipt)
			} else { // When an invalid transaction is supplied, there should be no receipt.
				require.Error(t, err)
				require.Nil(t, receipt)
			}

			// Fetch the balances after sending
			balanceSender, err = rpcClient.Eth().GetBalance(
				ethgo.Address(testCase.sender),
				ethgo.Latest,
			)
			require.NoError(t, err)

			balanceReceiver, err = rpcClient.Eth().GetBalance(
				ethgo.Address(testCase.recipient),
				ethgo.Latest,
			)
			require.NoError(t, err)

			expectedSenderBalance := previousSenderBalance
			expectedReceiverBalance := previousReceiverBalance
			if testCase.shouldSucceed {
				fee := new(big.Int).Mul(
					big.NewInt(int64(receipt.GasUsed)),
					txn.GasPrice,
				)

				expectedSenderBalance = previousSenderBalance.Sub(
					previousSenderBalance,
					new(big.Int).Add(testCase.amount, fee),
				)

				expectedReceiverBalance = previousReceiverBalance.Add(
					previousReceiverBalance,
					testCase.amount,
				)
			}

			// Check the balances
			require.Equalf(t,
				expectedSenderBalance,
				balanceSender,
				"Sender balance incorrect")
			require.Equalf(t,
				expectedReceiverBalance,
				balanceReceiver,
				"Receiver balance incorrect")
		})
	}
}
