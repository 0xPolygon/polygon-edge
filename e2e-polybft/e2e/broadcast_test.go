package e2e

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/types"
)

// Test_Broadcast sends two transactions (legacy and dynamic fees) to the cluster with the 1 amount of eth
// and checks that all cluster nodes have the recipient balance updated.
func Test_Broadcast(t *testing.T) {
	t.Parallel()

	const (
		sendAmount = int64(1)
	)

	// Create recipient key
	key, err := wallet.GenerateKey()
	assert.NoError(t, err)
	recipient := key.Address()
	t.Logf("Receipient %s\n", recipient)

	// Create pre-mined balance for sender
	sender, err := wallet.GenerateKey()
	require.NoError(t, err)

	// First account should have some matics premined
	cluster := framework.NewTestCluster(t, 5,
		framework.WithPremine(types.Address(sender.Address())),
	)
	defer cluster.Stop()

	// Wait until the cluster is up and running
	require.NoError(t, cluster.WaitForBlock(2, 1*time.Minute))

	client := cluster.Servers[0].JSONRPC().Eth()

	gasPrice, err := client.GasPrice()
	require.NoError(t, err)

	// Send legacy transaction
	legacyTxn := &ethgo.Transaction{
		Value:    big.NewInt(sendAmount),
		To:       &recipient,
		GasPrice: gasPrice,
		Gas:      21000,
		Nonce:    0,
	}
	sendTransaction(t, client, sender, legacyTxn)

	// Send dynamic fees transaction
	dynamicFeesTxn := &ethgo.Transaction{
		Value:                big.NewInt(sendAmount),
		To:                   &recipient,
		Gas:                  21000,
		Nonce:                1,
		MaxFeePerGas:         big.NewInt(1000000000),
		MaxPriorityFeePerGas: big.NewInt(100000000),
	}
	sendTransaction(t, client, sender, dynamicFeesTxn)

	// Wait until the balance has changed on all nodes in the cluster
	err = cluster.WaitUntil(time.Second*30, func() bool {
		for _, srv := range cluster.Servers {
			balance, err := srv.WaitForNonZeroBalance(recipient, time.Second*5)
			assert.NoError(t, err)
			if balance != nil && balance.BitLen() > 0 {
				assert.Equal(t, new(big.Int).Add(legacyTxn.Value, dynamicFeesTxn.Value), balance)
			} else {
				return false
			}
		}

		return true
	})
	assert.NoError(t, err)
}
