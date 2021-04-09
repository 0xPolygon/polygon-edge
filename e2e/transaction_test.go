package e2e

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"

	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
)

func TestTransaction(t *testing.T) {

	clt, err := jsonrpc.NewClient("http://127.0.0.1:8545")
	if err != nil {
		t.Fatal(err)
	}

	eth := clt.Eth()
	fmt.Println(eth.BlockNumber())

	root, err := eth.SendTransaction(&web3.Transaction{
		From:     web3.HexToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0b"),
		To:       web3.HexToAddress("0x9bd03347a977e4deb0a0ad685f8385f264524b0a").String(),
		GasPrice: 10000,
		Gas:      10000000000,
		Value:    big.NewInt(10000),
	})
	assert.NoError(t, err)
	fmt.Println(root)
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
