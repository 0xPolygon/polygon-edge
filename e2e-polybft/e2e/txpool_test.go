package e2e

import (
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestE2E_TxPool_Transfer(t *testing.T) {
	// premine an account in the genesis file
	sender, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, sender.Address())),
		framework.WithPremine(types.Address(sender.Address())),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}))
	defer cluster.Stop()

	cluster.WaitForReady(t)

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

			txn := &ethgo.Transaction{
				From:  sender.Address(),
				To:    &to,
				Gas:   30000, // enough to send a transfer
				Value: big.NewInt(int64(sendAmount)),
				Nonce: uint64(i),
			}

			// Send every second transaction as a dynamic fees one
			if i%2 == 0 {
				txn.Type = ethgo.TransactionDynamicFee
				txn.MaxFeePerGas = big.NewInt(1000000000)
				txn.MaxPriorityFeePerGas = big.NewInt(100000000)
			} else {
				txn.Type = ethgo.TransactionLegacy
				txn.GasPrice = ethgo.Gwei(2).Uint64()
			}

			sendTransaction(t, client, sender, txn)
		}(i, receivers[i])
	}

	wg.Wait()

	err = cluster.WaitUntil(2*time.Minute, 2*time.Second, func() bool {
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
	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, premine.Address())),
		framework.WithPremine(types.Address(premine.Address())),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	waitUntilBalancesChanged := func(acct ethgo.Address) error {
		err := cluster.WaitUntil(30*time.Second, 2*time.Second, func() bool {
			balance, err := client.GetBalance(acct, ethgo.Latest)
			if err != nil {
				return true
			}

			return balance.Cmp(big.NewInt(0)) > 0
		})

		return err
	}

	populateTxFees := func(txn *ethgo.Transaction, i int) {
		if i%2 == 0 {
			txn.Type = ethgo.TransactionDynamicFee
			txn.MaxFeePerGas = big.NewInt(1000000000)
			txn.MaxPriorityFeePerGas = big.NewInt(1000000000)
		} else {
			txn.Type = ethgo.TransactionLegacy
			txn.GasPrice = ethgo.Gwei(1).Uint64()
		}
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

	const sendAmount = 3000000

	// We are going to fund the accounts in linear fashion:
	// A (premined account) -> B -> C -> D -> E
	// At the end, all of them (except the premined account) will have the same `sendAmount`
	// of balance.
	for i := 1; i < num; i++ {
		// we have to send enough value to account `i` so that it has enough to fund
		// its child i+1 (cover costs + send amounts).
		// This means that since gasCost and sendAmount are fixed, account C must receive gasCost * 2
		// (to cover two more transfers C->D and D->E) + sendAmount * 3 (one bundle for each C,D and E).
		recipient := receivers[i].Address()
		txn := &ethgo.Transaction{
			Value: big.NewInt(int64(sendAmount * (num - i))),
			To:    &recipient,
			Gas:   21000,
		}

		// Populate fees fields for the current transaction
		populateTxFees(txn, i-1)

		// Add remaining fees to finish the cycle
		gasCostTotal := new(big.Int).Mul(txCost(txn), new(big.Int).SetInt64(int64(num-i-1)))
		txn.Value = txn.Value.Add(txn.Value, gasCostTotal)

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

func TestE2E_TxPool_TransactionWithHeaderInstructions(t *testing.T) {
	sidechainKey, err := wallet.GenerateKey()
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, sidechainKey.Address())),
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

// TestE2E_TxPool_BroadcastTransactions sends several transactions (legacy and dynamic fees) to the cluster
// with the 1 amount of eth and checks that all cluster nodes have the recipient balance updated.
func TestE2E_TxPool_BroadcastTransactions(t *testing.T) {
	var (
		sendAmount = ethgo.Ether(1)
	)

	const (
		txNum = 10
	)

	// Create recipient key
	key, err := wallet.GenerateKey()
	assert.NoError(t, err)

	recipient := key.Address()

	t.Logf("Recipient %s\n", recipient)

	// Create pre-mined balance for sender
	sender, err := wallet.GenerateKey()
	require.NoError(t, err)

	// First account should have some matics premined
	cluster := framework.NewTestCluster(t, 5,
		framework.WithNativeTokenConfig(fmt.Sprintf(framework.NativeTokenMintableTestCfg, sender.Address())),
		framework.WithPremine(types.Address(sender.Address())),
		framework.WithBurnContract(&polybft.BurnContractInfo{BlockNumber: 0, Address: types.ZeroAddress}),
	)
	defer cluster.Stop()

	// Wait until the cluster is up and running
	cluster.WaitForReady(t)

	client := cluster.Servers[0].JSONRPC().Eth()

	sentAmount := new(big.Int)
	nonce := uint64(0)

	for i := 0; i < txNum; i++ {
		txn := &ethgo.Transaction{
			Value: sendAmount,
			To:    &recipient,
			Gas:   21000,
			Nonce: nonce,
		}

		if i%2 == 0 {
			txn.Type = ethgo.TransactionDynamicFee
			txn.MaxFeePerGas = big.NewInt(1000000000)
			txn.MaxPriorityFeePerGas = big.NewInt(100000000)
		} else {
			txn.Type = ethgo.TransactionLegacy
			txn.GasPrice = ethgo.Gwei(2).Uint64()
		}

		sendTransaction(t, client, sender, txn)
		sentAmount = sentAmount.Add(sentAmount, txn.Value)
		nonce++
	}

	// Wait until the balance has changed on all nodes in the cluster
	err = cluster.WaitUntil(time.Minute, time.Second*3, func() bool {
		for _, srv := range cluster.Servers {
			balance, err := srv.WaitForNonZeroBalance(recipient, time.Second*10)
			assert.NoError(t, err)
			if balance != nil && balance.BitLen() > 0 {
				assert.Equal(t, sentAmount, balance)
			} else {
				return false
			}
		}

		return true
	})
	assert.NoError(t, err)
}

// sendTransaction is a helper function which signs transaction with provided private key and sends it
func sendTransaction(t *testing.T, client *jsonrpc.Eth, sender *wallet.Key, txn *ethgo.Transaction) {
	t.Helper()

	chainID, err := client.ChainID()
	require.NoError(t, err)

	if txn.Type == ethgo.TransactionDynamicFee {
		txn.ChainID = chainID
	}

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	signedTxn, err := signer.SignTx(txn, sender)
	require.NoError(t, err)

	txnRaw, err := signedTxn.MarshalRLPTo(nil)
	require.NoError(t, err)

	_, err = client.SendRawTransaction(txnRaw)
	require.NoError(t, err)
}

func txCost(t *ethgo.Transaction) *big.Int {
	var factor *big.Int

	if t.Type == ethgo.TransactionDynamicFee {
		factor = new(big.Int).Set(t.MaxFeePerGas)
	} else {
		factor = new(big.Int).SetUint64(t.GasPrice)
	}

	return new(big.Int).Mul(factor, new(big.Int).SetUint64(t.Gas))
}
