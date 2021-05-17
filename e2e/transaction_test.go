package e2e

import (
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/wallet"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
)

var (
	addr0 = types.Address{}
	addr1 = types.Address{0x1}
)

func TestSignedTransaction(t *testing.T) {
	fr := framework.NewTestServerFromGenesis(t)

	// 0xdf7fd4830f4cc1440b469615e9996e9fde92608f
	var privKeyRaw = "0x4b2216c76f1b4c60c44d41986863e7337bc1a317d6a9366adfd8966fe2ac05f6"
	key, err := wallet.NewWalletFromPrivKey(hex.MustDecodeHex(privKeyRaw))
	assert.NoError(t, err)

	clt := fr.JSONRPC()

	// check there is enough balance
	balance, err := clt.Eth().GetBalance(key.Address(), web3.Latest)
	assert.NoError(t, err)

	fmt.Println("-- balance --")
	fmt.Println(balance)

	// latest nonce
	lastNonce, err := clt.Eth().GetNonce(key.Address(), web3.Latest)
	assert.NoError(t, err)

	target := web3.Address{0x1}

	// get the chain id and the signer
	chainID, err := clt.Eth().ChainID()
	assert.NoError(t, err)

	signer := wallet.NewEIP155Signer(chainID.Uint64())

	for i := 0; i < 5; i++ {
		txn := &web3.Transaction{
			From:     key.Address(),
			To:       &target,
			GasPrice: 10000,
			Gas:      1000000,
			Value:    big.NewInt(10000),
			Nonce:    lastNonce + uint64(i),
		}
		txn, err = signer.SignTx(txn, key)
		assert.NoError(t, err)

		data := txn.MarshalRLP()
		hash, err := clt.Eth().SendRawTransaction(data)
		assert.NoError(t, err)

		fr.WaitForReceipt(hash)
	}
}

func TestPreminedBalance(t *testing.T) {
	validAccounts := []struct {
		address types.Address
		balance *big.Int
	}{
		{types.StringToAddress("1"), big.NewInt(0)},
		{types.StringToAddress("2"), big.NewInt(20)},
	}

	testTable := []struct {
		name       string
		address    types.Address
		balance    *big.Int
		shouldFail bool
	}{
		{
			"Account with 0 balance",
			validAccounts[0].address,
			validAccounts[0].balance,
			false,
		},
		{
			"Account with valid balance",
			validAccounts[1].address,
			validAccounts[1].balance,
			false,
		},
		{
			"Account not in genesis",
			types.StringToAddress("3"),
			big.NewInt(0),
			true,
		},
	}

	srv := framework.NewTestServer(t, func(config *framework.TestServerConfig) {
		for _, acc := range validAccounts {
			config.Premine(acc.address, acc.balance)
		}
	})
	defer srv.Stop()

	rpcClient := srv.JSONRPC()

	for _, testCase := range testTable {

		t.Run(testCase.name, func(t *testing.T) {

			balance, err := rpcClient.Eth().GetBalance(web3.Address(testCase.address), web3.Latest)

			if err != nil && !testCase.shouldFail {
				assert.Failf(t, "Uncaught error", err.Error())
			}

			if !testCase.shouldFail {
				assert.Equalf(t, testCase.balance, balance, "Balances don't match")
			}
		})
	}
}

func ethToWei(ethValue int64) *big.Int {
	return new(big.Int).Mul(
		big.NewInt(ethValue),
		new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
}

func TestEthTransfer(t *testing.T) {
	validAccounts := []struct {
		address types.Address
		balance *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("1"),
			ethToWei(50), // 50 ETH
		},
		// Empty account
		{
			types.StringToAddress("2"),
			big.NewInt(0)},
		// Valid account #2
		{
			types.StringToAddress("3"),
			ethToWei(10), // 10 ETH
		},
	}

	testTable := []struct {
		name       string
		sender     types.Address
		recipient  types.Address
		amount     *big.Int
		shouldFail bool
	}{
		{
			// ACC #1 -> ACC #3
			"Valid ETH transfer #1",
			validAccounts[0].address,
			validAccounts[2].address,
			ethToWei(10), // 10 ETH
			false,
		},
		{
			// ACC #2 -> ACC #3
			"Invalid ETH transfer",
			validAccounts[1].address,
			validAccounts[2].address,
			ethToWei(100),
			true,
		},
		{
			// ACC #2 -> ACC #1
			"Valid ETH transfer #2",
			validAccounts[2].address,
			validAccounts[1].address,
			ethToWei(5), // 5 ETH
			false,
		},
	}

	srv := framework.NewTestServer(t, func(config *framework.TestServerConfig) {
		config.SetDev(true)
		config.SetSeal(true)

		for _, acc := range validAccounts {
			config.Premine(acc.address, acc.balance)
		}
	})
	defer srv.Stop()

	checkSenderReceiver := func(errSender error, errReceiver error, t *testing.T) {
		if errSender != nil || errReceiver != nil {
			if errSender != nil {
				assert.Failf(t, "Uncaught error", errSender.Error())
			} else {
				assert.Failf(t, "Uncaught error", errReceiver.Error())
			}
		}
	}

	rpcClient := srv.JSONRPC()

	for _, testCase := range testTable {

		t.Run(testCase.name, func(t *testing.T) {

			preSendData := struct {
				previousSenderBalance   *big.Int
				previousReceiverBalance *big.Int
			}{
				previousSenderBalance:   big.NewInt(0),
				previousReceiverBalance: big.NewInt(0),
			}

			// Fetch the balances before sending
			balanceSender, errSender := rpcClient.Eth().GetBalance(
				web3.Address(testCase.sender),
				web3.Latest,
			)

			balanceReceiver, errReceiver := rpcClient.Eth().GetBalance(
				web3.Address(testCase.recipient),
				web3.Latest,
			)

			checkSenderReceiver(errSender, errReceiver, t)

			// Set the preSend balances
			preSendData.previousSenderBalance = balanceSender
			preSendData.previousReceiverBalance = balanceReceiver

			toAddress := web3.Address(testCase.recipient)

			gasPrice := uint64(1048576)

			// Create the transaction
			txnObject := &web3.Transaction{
				From:     web3.Address(testCase.sender),
				To:       &toAddress,
				GasPrice: gasPrice,
				Gas:      1000000,
				Value:    testCase.amount,
			}

			// Do the transfer
			txnHash, err := rpcClient.Eth().SendTransaction(txnObject)

			// Error checking
			if err != nil && !testCase.shouldFail {
				assert.Failf(t, "Uncaught error", err.Error())
			}

			// Wait until the transaction goes through
			time.Sleep(5 * time.Second)

			receipt, err := rpcClient.Eth().GetTransactionReceipt(txnHash)
			if receipt == nil {
				t.Fatalf("Unable to fetch receipt")
			}

			// Fetch the balances after sending
			balanceSender, errSender = rpcClient.Eth().GetBalance(
				web3.Address(testCase.sender),
				web3.Latest,
			)

			balanceReceiver, errReceiver = rpcClient.Eth().GetBalance(
				web3.Address(testCase.recipient),
				web3.Latest,
			)

			assert.IsTypef(t, web3.Hash{}, txnHash, "Return type mismatch")

			checkSenderReceiver(errSender, errReceiver, t)

			expandedGasUsed := new(big.Int).Mul(
				big.NewInt(int64(receipt.GasUsed)),
				big.NewInt(int64(gasPrice)),
			)

			if !testCase.shouldFail {
				spentAmount := new(big.Int).Add(testCase.amount, expandedGasUsed)

				// Check the sender balance
				assert.Equalf(t,
					new(big.Int).Sub(preSendData.previousSenderBalance, spentAmount),
					balanceSender,
					"Sender balance incorrect")

				// Check the receiver balance
				assert.Equalf(t,
					new(big.Int).Add(preSendData.previousReceiverBalance, testCase.amount).String(),
					balanceReceiver.String(),
					"Receiver balance incorrect")
			}
		})
	}
}
