package e2e

import (
	"fmt"
	"os"
	"path"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi/artifact"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
)

func TestE2E_AccountAbstraction(t *testing.T) {
	const (
		amount             = 15
		customSecretPrefix = "other-"
	)

	receiver1 := types.StringToAddress("0xff00ff00ff00cc00bb11893")
	receiver2 := types.StringToAddress("0xff00ff00ff00cc00bb11894")

	deployerAccount, err := wallet.GenerateAccount()
	require.NoError(t, err)

	aaInvoker, err := artifact.DecodeArtifact([]byte(contractsapi.AccountAbstractionInvokerArtifact))
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithSecretsCallback(func(_ []types.Address, clst *framework.TestCluster) {
			addresses, err := clst.InitSecrets(customSecretPrefix, 2) // generate two additional accounts
			require.NoError(t, err)

			userBalance := fmt.Sprintf("%s:%d", addresses[0], amount*5)
			relayerBalance := fmt.Sprintf("%s:%d", addresses[1], ethgo.Ether(1))
			deployerBalance := fmt.Sprintf("%s:%d", deployerAccount.Ecdsa.Address(), ethgo.Ether(1))
			clst.Config.Premine = append(clst.Config.Premine, userBalance, relayerBalance, deployerBalance)
		}))
	defer func() {
		cluster.Stop()
		os.RemoveAll(path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s1", customSecretPrefix)))
		os.RemoveAll(path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s2", customSecretPrefix)))
	}()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	cluster.WaitForReady(t)

	// deploy account abstraction smart contract
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		Input: aaInvoker.Bytecode,
	}, deployerAccount.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	// start aa relayer service
	aaRelayer := framework.NewTestAARelayer(
		t,
		&framework.TestAARelayerConfig{
			Addr:           "127.0.0.1:8198",
			JSONRPCAddress: cluster.Servers[0].JSONRPCAddr(),
			Stdout:         cluster.Config.GetStdout("aarelayer"),
			Binary:         cluster.Config.Binary,
			DBPath:         path.Join(cluster.Config.TmpDir, "aarelayer.db"),
			InvokerAddress: types.Address(receipt.ContractAddress),
			DataDir:        path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s2", customSecretPrefix)),
		})
	defer aaRelayer.Stop()

	time.Sleep(time.Second * 10) // wait some time for the aa relayer rest server to start

	// send two aa tx to receiver1 (3 payloads) and to receiver2 one aa tx
	addresses := []types.Address{receiver1, receiver2, receiver1}

	for i, address := range addresses {
		txs := []string{
			fmt.Sprintf("%s:%d:%d", address, amount, 21000),
		}

		if i == 0 {
			// first time send two payloads
			txs = append(txs, fmt.Sprintf("%s:%d:%d", address, amount*2, 21000))
		}

		require.NoError(t, aaRelayer.AASendTx(
			path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s1", customSecretPrefix)),
			uint64(len(addresses)-i-1), // let the aa tx pool to sort nonces out
			txs,
			true,
			cluster.Config.GetStdout(fmt.Sprintf("aarelayersendtx%d", i)),
		))

		time.Sleep(5 * time.Second)
	}

	// check if balances of receiver1 and receiver2 are correct
	require.NoError(t, cluster.WaitUntil(time.Second*180, time.Second*10, func() bool {
		val, err := cluster.Servers[0].JSONRPC().Eth().GetBalance(ethgo.Address(receiver2), ethgo.Latest)
		if err != nil || val.Uint64() != amount {
			return false
		}

		val, err = cluster.Servers[0].JSONRPC().Eth().GetBalance(ethgo.Address(receiver1), ethgo.Latest)

		return err == nil && val.Uint64() == amount*4
	}))
}
