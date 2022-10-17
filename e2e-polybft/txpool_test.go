package e2e

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

func TestE2E_TxPool_Transfer(t *testing.T) {
	// premine an account in the genesis file
	sender, err := wallet.GenerateKey()
	assert.NoError(t, err)

	cluster := framework.NewTestCluster(
		t,
		"txpool_transfer",
		5,
		framework.WithPremine(common.Address(sender.Address())),
	)
	defer cluster.Stop()

	assert.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	client := cluster.Servers[0].JSONRPC().Eth()

	// send n value transfers
	sendTransaction := func(nonce uint64, receiver ethgo.Address, value *big.Int) ethgo.Hash {
		// estimate gas price
		gasPrice, err := client.GasPrice()
		assert.NoError(t, err)

		chainID, err := client.ChainID()
		assert.NoError(t, err)

		// send transaction
		rawTxn := &ethgo.Transaction{
			From:     sender.Address(),
			To:       &receiver,
			GasPrice: gasPrice,
			Gas:      30000, // enough to send a transfer
			Value:    value,
			Nonce:    nonce,
		}

		signer := wallet.NewEIP155Signer(chainID.Uint64())
		signedTxn, err := signer.SignTx(rawTxn, sender)
		assert.NoError(t, err)

		txnRaw, err := signedTxn.MarshalRLPTo(nil)
		assert.NoError(t, err)

		hash, err := client.SendRawTransaction(txnRaw)
		assert.NoError(t, err)

		return hash
	}

	sendAmount := 1
	num := 20

	receivers := []ethgo.Address{}

	for i := 0; i < num; i++ {
		key, err := wallet.GenerateKey()
		assert.NoError(t, err)

		receivers = append(receivers, key.Address())
	}

	var wg sync.WaitGroup
	for i := 0; i < num; i++ {
		wg.Add(1)

		go func(i int, to ethgo.Address) {
			defer wg.Done()

			sendTransaction(uint64(i), to, big.NewInt(int64(sendAmount)))
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
				return true
			}
		}

		return false
	})
	assert.NoError(t, err)
}

// First account send some amount to second one and then second one to third account
func TestE2E_TxPool_Transfer_Linear(t *testing.T) {
	premine, err := wallet.GenerateKey()
	assert.NoError(t, err)

	// first account should have some matics premined
	cluster := framework.NewTestCluster(
		t,
		"txpool_transfer_linear",
		5,
		framework.WithPremine(common.Address(premine.Address())),
	)
	defer cluster.Stop()

	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	client := cluster.Servers[0].JSONRPC().Eth()

	// estimate gas price
	gasPrice, err := client.GasPrice()
	require.NoError(t, err)

	// send n value transfers
	sendTransaction := func(from *wallet.Key, receiver ethgo.Address, value *big.Int) {
		chainID, err := client.ChainID()
		require.NoError(t, err)

		// send transaction
		rawTxn := &ethgo.Transaction{
			From:     from.Address(),
			To:       &receiver,
			GasPrice: gasPrice,
			Gas:      21000,
			Value:    value,
			Nonce:    0,
		}

		signer := wallet.NewEIP155Signer(chainID.Uint64())
		signedTxn, err := signer.SignTx(rawTxn, from)
		require.NoError(t, err)

		txnRaw, err := signedTxn.MarshalRLPTo(nil)
		require.NoError(t, err)

		_, err = client.SendRawTransaction(txnRaw)
		require.NoError(t, err)
	}

	waitUntilBalancesChanged := func(acct ethgo.Address) error {
		err := cluster.WaitUntil(30*time.Second, func() bool {
			balance, err := client.GetBalance(acct, ethgo.Latest)
			if err != nil {
				return true
			}

			if balance.Cmp(big.NewInt(0)) == 0 {
				return true
			}

			return false
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
	// A (premise) -> B -> C -> D -> E
	// At the end, all of them (except the premised) will have the same `sendAmount`
	// of balance.
	for i := 1; i < num; i++ {
		// we have to send enough value to account `i` so that it has enough to fund
		// its child i+1 (cover costs + send amounts).
		// This means that since gasCost and sendAmount are fixed, account C must receive gasCost * 2
		// (to cover two more transfers C->D and D->E) + sendAmount * 3 (one bundle for each C,D and E).
		amount := gasCost*(num-i-1) + sendAmount*(num-i)
		sendTransaction(receivers[i-1], receivers[i].Address(), big.NewInt(int64(amount)))

		err := waitUntilBalancesChanged(receivers[i].Address())
		assert.NoError(t, err)
	}

	for i := 1; i < num; i++ {
		balance, err := client.GetBalance(receivers[i].Address(), ethgo.Latest)
		assert.NoError(t, err)
		assert.Equal(t, uint64(sendAmount), balance.Uint64())
	}
}
