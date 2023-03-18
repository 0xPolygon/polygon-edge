package e2e

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

func TestE2E_TxPool_Transfer(t *testing.T) {
	// premine an account in the genesis file
	sender, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5, framework.WithPremine(types.Address(sender.Address())))
	defer cluster.Stop()

	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	client := cluster.Servers[0].JSONRPC().Eth()

	sendAmount := 1
	num := 20

	receivers := []ethgo.Address{}

	for i := 0; i < num; i++ {
		key, err := wallet.GenerateKey()
		require.NoError(t, err)

		receivers = append(receivers, key.Address())
	}

	var wg sync.WaitGroup
	for i := 0; i < num; i++ {
		wg.Add(1)

		go func(i int, to ethgo.Address) {
			defer wg.Done()

			gasPrice, err := client.GasPrice()
			require.NoError(t, err)

			txn := &ethgo.Transaction{
				From:     sender.Address(),
				To:       &to,
				GasPrice: gasPrice,
				Gas:      30000, // enough to send a transfer
				Value:    big.NewInt(int64(sendAmount)),
				Nonce:    uint64(i),
			}
			sendTransaction(t, client, sender, txn)
		}(i, receivers[i])
	}

	wg.Wait()

	err = cluster.WaitUntil(2*time.Minute, func() bool {
		for _, receiver := range receivers {
			balance, err := client.GetBalance(receiver, ethgo.Latest)
			if err != nil {
				return true
			}
			t.Logf("Balance %s %s", receiver, balance)
			if balance.Uint64() != uint64(sendAmount) {
				return false
			}
		}

		return true
	})
	require.NoError(t, err)
}

// First account send some amount to second one and then second one to third account
func TestE2E_TxPool_Transfer_Linear(t *testing.T) {
	premine, err := wallet.GenerateKey()
	require.NoError(t, err)

	// first account should have some matics premined
	cluster := framework.NewTestCluster(t, 5, framework.WithPremine(types.Address(premine.Address())))
	defer cluster.Stop()

	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	client := cluster.Servers[0].JSONRPC().Eth()

	// estimate gas price
	gasPrice, err := client.GasPrice()
	require.NoError(t, err)

	waitUntilBalancesChanged := func(acct ethgo.Address) error {
		err := cluster.WaitUntil(30*time.Second, func() bool {
			balance, err := client.GetBalance(acct, ethgo.Latest)
			if err != nil {
				return true
			}

			return balance.Cmp(big.NewInt(0)) > 0
		})

		return err
	}

	num := 4
	receivers := []*wallet.Key{
		premine,
	}

	for i := 0; i < num-1; i++ {
		key, err := wallet.GenerateKey()
		assert.NoError(t, err)

		receivers = append(receivers, key)
	}

	// Gas cost is always the same since value transfers are deterministic (21k gas).
	// Then, the total gas cost required to make a transfer is 21k multiplied by
	// the selected gas price.
	gasCost := int(21000 * gasPrice)
	sendAmount := 3000000

	// We are going to fund the accounts in linear fashion:
	// A (premined account) -> B -> C -> D -> E
	// At the end, all of them (except the premined account) will have the same `sendAmount`
	// of balance.
	for i := 1; i < num; i++ {
		// we have to send enough value to account `i` so that it has enough to fund
		// its child i+1 (cover costs + send amounts).
		// This means that since gasCost and sendAmount are fixed, account C must receive gasCost * 2
		// (to cover two more transfers C->D and D->E) + sendAmount * 3 (one bundle for each C,D and E).
		amount := gasCost*(num-i-1) + sendAmount*(num-i)
		recipient := receivers[i].Address()
		txn := &ethgo.Transaction{
			Value:    big.NewInt(int64(amount)),
			To:       &recipient,
			GasPrice: gasPrice,
			Gas:      21000,
			Nonce:    0,
		}
		sendTransaction(t, client, receivers[i-1], txn)

		err := waitUntilBalancesChanged(receivers[i].Address())
		require.NoError(t, err)
	}

	for i := 1; i < num; i++ {
		balance, err := client.GetBalance(receivers[i].Address(), ethgo.Latest)
		require.NoError(t, err)
		require.Equal(t, uint64(sendAmount), balance.Uint64())
	}
}

func TestE2E_TxPool_TransactionWithHeaderInstuctions(t *testing.T) {
	sidechainKey, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithPremine(types.Address(sidechainKey.Address())),
	)
	defer cluster.Stop()

	require.NoError(t, cluster.WaitForBlock(1, 20*time.Second))

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	receipt, err := relayer.SendTransaction(&ethgo.Transaction{Input: contractsapi.TestWriteBlockMetadata.Bytecode}, sidechainKey)
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	receipt, err = ABITransaction(relayer, sidechainKey, contractsapi.TestWriteBlockMetadata, receipt.ContractAddress, "init", []interface{}{})
	require.NoError(t, err)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	require.NoError(t, cluster.WaitForBlock(10, 1*time.Minute))
}

// sendTransaction is a helper function which signs transaction with provided private key and sends it
func sendTransaction(t *testing.T, client *jsonrpc.Eth, sender *wallet.Key, txn *ethgo.Transaction) {
	t.Helper()

	chainID, err := client.ChainID()
	require.NoError(t, err)

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	signedTxn, err := signer.SignTx(txn, sender)
	require.NoError(t, err)

	txnRaw, err := signedTxn.MarshalRLPTo(nil)
	require.NoError(t, err)

	_, err = client.SendRawTransaction(txnRaw)
	require.NoError(t, err)
}
