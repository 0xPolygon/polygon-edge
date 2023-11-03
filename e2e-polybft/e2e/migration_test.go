package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb/opt"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	frameworkpolybft "github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	itrie "github.com/0xPolygon/polygon-edge/state/immutable-trie"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

func TestE2E_Migration(t *testing.T) {
	userKey, _ := wallet.GenerateKey()
	userAddr := userKey.Address()
	userKey2, _ := wallet.GenerateKey()
	userAddr2 := userKey2.Address()

	initialBalance := ethgo.Ether(10)
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.Premine(types.Address(userAddr), initialBalance)
	})
	srv := srvs[0]

	rpcClient := srv.JSONRPC()

	// Fetch the balances before sending
	balanceSender, err := rpcClient.Eth().GetBalance(
		userAddr,
		ethgo.Latest,
	)
	require.NoError(t, err)
	require.Equal(t, balanceSender.Cmp(initialBalance), 0)

	balanceReceiver, err := rpcClient.Eth().GetBalance(
		userAddr2,
		ethgo.Latest,
	)
	require.NoError(t, err)

	if balanceReceiver.Uint64() != 0 {
		t.Fatal("balanceReceiver is not 0")
	}

	relayer, err := txrelayer.NewTxRelayer(txrelayer.WithClient(rpcClient))
	require.NoError(t, err)

	//send transaction to user2
	sendAmount := ethgo.Gwei(10000)
	receipt, err := relayer.SendTransaction(&ethgo.Transaction{
		From:     userAddr,
		To:       &userAddr2,
		Gas:      1000000,
		Value:    sendAmount,
		GasPrice: ethgo.Gwei(2).Uint64(),
	}, userKey)
	require.NoError(t, err)
	require.NotNil(t, receipt)

	receipt, err = relayer.SendTransaction(&ethgo.Transaction{
		From:     userAddr,
		Gas:      1000000,
		GasPrice: ethgo.Gwei(2).Uint64(),
		Input:    contractsapi.TestWriteBlockMetadata.Bytecode,
	}, userKey)
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, uint64(types.ReceiptSuccess), receipt.Status)

	deployedContractBalance := receipt.ContractAddress

	initReceipt, err := ABITransaction(relayer, userKey, contractsapi.TestWriteBlockMetadata, receipt.ContractAddress, "init")
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, uint64(types.ReceiptSuccess), initReceipt.Status)

	// Fetch the balances after sending
	balanceSender, err = rpcClient.Eth().GetBalance(
		userAddr,
		ethgo.Latest,
	)
	require.NoError(t, err)

	balanceReceiver, err = rpcClient.Eth().GetBalance(
		userAddr2,
		ethgo.Latest,
	)
	require.NoError(t, err)
	require.Equal(t, sendAmount, balanceReceiver)

	block, err := rpcClient.Eth().GetBlockByNumber(ethgo.Latest, true)
	if err != nil {
		t.Fatal(err)
	}

	stateRoot := block.StateRoot

	path := filepath.Join(srvs[0].Config.RootDir, "trie")
	srvs[0].Stop()
	//hack for db closing. leveldb allow only one connection
	time.Sleep(time.Second)

	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	err = frameworkpolybft.RunEdgeCommand([]string{
		"regenesis",
		"--stateRoot", block.StateRoot.String(),
		"--source-path", path,
		"--target-path", tmpDir,
	}, os.Stdout)
	if err != nil {
		t.Fatal(err)
	}

	db, err := leveldb.OpenFile(tmpDir, &opt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}

	stateStorageNew := itrie.NewKV(db)

	copiedStateRoot, err := itrie.HashChecker(block.StateRoot.Bytes(), stateStorageNew)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, types.Hash(stateRoot), copiedStateRoot)

	err = db.Close()
	if err != nil {
		t.Fatal(err)
	}

	cluster := frameworkpolybft.NewTestCluster(t, 7,
		frameworkpolybft.WithNonValidators(2),
		frameworkpolybft.WithValidatorSnapshot(5),
		frameworkpolybft.WithTestRewardToken(),
		frameworkpolybft.WithGenesisState(tmpDir, types.Hash(stateRoot)),
	)
	defer cluster.Stop()

	cluster.WaitForReady(t)

	senderBalanceAfterMigration, err := cluster.Servers[0].JSONRPC().Eth().GetBalance(userAddr, ethgo.Latest)
	if err != nil {
		t.Fatal(err)
	}

	receiverBalanceAfterMigration, err := cluster.Servers[0].JSONRPC().Eth().GetBalance(userAddr2, ethgo.Latest)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, balanceSender, senderBalanceAfterMigration)
	require.Equal(t, balanceReceiver, receiverBalanceAfterMigration)

	deployedCode, err := cluster.Servers[0].JSONRPC().Eth().GetCode(deployedContractBalance, ethgo.Latest)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, deployedCode, *common.EncodeBytes(contractsapi.TestWriteBlockMetadata.DeployedBytecode))
	require.NoError(t, cluster.WaitForBlock(10, 1*time.Minute))

	//stop last node of validator and non-validator
	cluster.Servers[4].Stop()
	cluster.Servers[6].Stop()

	require.NoError(t, cluster.WaitForBlock(15, time.Minute))

	//wait sync of that nodes
	cluster.Servers[4].Start()
	cluster.Servers[6].Start()
	require.NoError(t, cluster.WaitForBlock(20, time.Minute))

	//stop all nodes
	for i := range cluster.Servers {
		cluster.Servers[i].Stop()
	}

	time.Sleep(time.Second)

	for i := range cluster.Servers {
		cluster.Servers[i].Start()
	}

	require.NoError(t, cluster.WaitForBlock(25, time.Minute))

	// add new node
	_, err = cluster.InitSecrets("test-chain-8", 1)
	require.NoError(t, err)

	cluster.InitTestServer(t, "test-chain-8", cluster.Bridge.JSONRPCAddr(), frameworkpolybft.None)
	require.NoError(t, cluster.WaitForBlock(33, time.Minute))
}
