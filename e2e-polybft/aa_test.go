package e2e

import (
	"fmt"
	"math/big"
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

	someRandomAddress1 := types.StringToAddress("0xff00ff00ff00cc00bb11893")
	someRandomAddress2 := types.StringToAddress("0xff00ff00ff00cc00bb11894")
	deployerAccount := wallet.GenerateAccount()

	aaInvoker, err := artifact.DecodeArtifact([]byte(contractsapi.AccountAbstractionInvokerArtifact))
	require.NoError(t, err)

	cluster := framework.NewTestCluster(t, 4,
		framework.WithSecretsCallback(func(_ []types.Address, cfg *framework.TestClusterConfig, clst *framework.TestCluster) {
			addresses, err := clst.InitSecrets(customSecretPrefix, 2) // generate two additional accounts
			require.NoError(t, err)

			userBalance := fmt.Sprintf("%s:%d", addresses[0], amount*3)
			relayerBalance := fmt.Sprintf("%s:%d", addresses[1], 0xFF00FF00000000)
			deployerBalance := fmt.Sprintf("%s:%d", deployerAccount.Ecdsa.Address(), 0xFF00FF00000000)
			cfg.Premine = append(cfg.Premine, userBalance, relayerBalance, deployerBalance)
		}))
	defer func() {
		cluster.Stop()
		os.RemoveAll(path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s1", customSecretPrefix)))
		os.RemoveAll(path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s2", customSecretPrefix)))
	}()

	txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(cluster.Servers[0].JSONRPCAddr()))
	require.NoError(t, err)

	cluster.WaitForBlock(2, time.Second*20)

	// deploy account abstraction smart contract
	receipt, err := txRelayer.SendTransaction(&ethgo.Transaction{
		Input: aaInvoker.Bytecode,
	}, deployerAccount.Ecdsa)
	require.NoError(t, err)
	require.Equal(t, receipt.Status, uint64(types.ReceiptSuccess))

	cluster.WaitForBlock(6, time.Second*60)

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

	cluster.WaitForBlock(10, time.Second*40)

	// send to someRandomAddress1 some amount twice, and to someRandomAddress2 once same amount
	for i, address := range []types.Address{someRandomAddress1, someRandomAddress2, someRandomAddress1} {
		require.NoError(t, aaRelayer.AASendTx(
			path.Join(cluster.Config.TmpDir, fmt.Sprintf("%s1", customSecretPrefix)),
			address,
			big.NewInt(amount),
			big.NewInt(21000),
			uint64(i),
		))

		time.Sleep(time.Second * 2)
	}

	require.NoError(t, cluster.WaitUntil(time.Second*90, func() bool {
		// check if balances of someRandomAddress1 and someRandomAddress2 are correct
		val, err := cluster.Servers[0].JSONRPC().Eth().GetBalance(ethgo.Address(someRandomAddress2), ethgo.Latest)
		if err != nil || val.Uint64() != amount {
			return false
		}

		val, err = cluster.Servers[0].JSONRPC().Eth().GetBalance(ethgo.Address(someRandomAddress1), ethgo.Latest)

		return err == nil && val.Uint64() == amount*2
	}))
}
