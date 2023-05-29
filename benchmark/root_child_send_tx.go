package benchmark

import (
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/wallet"
)

// RootChildSendTxSetUp sets environment for execution of sentTx test cases on both root and child chains and
// returns test cases and clean up fn.
// The rootJSONRPC, childJSONRPC, privateKey and startCluster params are used to configure the testing environment.
// If startCluster is false, then the local cluster will not be started and the provided addresses
// will be used as the endpoints to the root and child chains.
// If startCluster is set to true, the local cluster will be started automatically.
// If the private key is specified, it will be used as the transaction sender.
// Otherwise, the local cluster will generate a sender key.
// If startCluster is set to false, then the sender must have enough funds for sending transactions.
func RootChildSendTxSetUp(b *testing.B, rootNodeAddr, childNodeAddr,
	privateKey string, startCluster bool) ([]TxTestCase, func()) {
	b.Helper()
	// check if test is called with the root and child node addresses and private key set.
	// if that is the case use that json rpc addresses, otherwise run the cluster
	require.True(b, startCluster || rootNodeAddr != "" && childNodeAddr != "" && privateKey != "")

	var sender ethgo.Key
	// if the privateKey flag is set then recover the key, otherwise recover the key
	if privateKey != "" {
		sender = getPrivateKey(b, privateKey)
	} else {
		var err error
		sender, err = wallet.GenerateKey()
		require.NoError(b, err)
	}

	senderEthAddr := sender.Address()

	// start the cluster if needed
	cleanUpFn := func() {}

	if startCluster {
		b.Log("Starting the cluster...")
		cluster := framework.NewTestCluster(b, 5,
			framework.WithPremine(types.Address(senderEthAddr)),
			framework.WithEpochSize(5))

		cleanUpFn = func() { cluster.Stop() }

		cluster.WaitForReady(b)
		rootNodeAddr = cluster.Bridge.JSONRPCAddr()
		childNodeAddr = cluster.Servers[0].JSONRPCAddr()
	}

	//create tx relayers
	rootTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(rootNodeAddr))
	require.NoError(b, err)

	childTxRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(childNodeAddr))
	require.NoError(b, err)

	if startCluster {
		// fund sender on root chain to be able to deploy contracts and execute transactions
		txn := &ethgo.Transaction{To: &senderEthAddr, Value: ethgo.Ether(1)}
		receipt, err := rootTxRelayer.SendTransactionLocal(txn)
		require.NoError(b, err)
		require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
	}

	// deploy contracts
	singleContChildAddr, singleContRootAddr := deployContractOnRootAndChild(b, childTxRelayer, rootTxRelayer,
		sender, contractsapi.TestBenchmarkSingle.Bytecode)
	multiAContChildAddr, multiAContRootAddr := deployContractOnRootAndChild(b, childTxRelayer, rootTxRelayer,
		sender, contractsapi.TestBenchmarkA.Bytecode)
	multiBContChildAddr, multiBContRootAddr := deployContractOnRootAndChild(b, childTxRelayer, rootTxRelayer,
		sender, contractsapi.TestBenchmarkB.Bytecode)
	multiCContChildAddr, multiCContRootAddr := deployContractOnRootAndChild(b, childTxRelayer, rootTxRelayer,
		sender, contractsapi.TestBenchmarkC.Bytecode)

	// set callee contract addresses for multi call contracts (A->B->C)
	// set B contract address in A contract
	setContractDependencyAddress(b, childTxRelayer, multiAContChildAddr, multiBContChildAddr,
		multiContSetAAddrFunc, sender)
	setContractDependencyAddress(b, rootTxRelayer, multiAContRootAddr, multiBContRootAddr,
		multiContSetAAddrFunc, sender)
	// set C contract address in B contract
	setContractDependencyAddress(b, childTxRelayer, multiBContChildAddr, multiCContChildAddr,
		multiContSetBAddrFunc, sender)
	setContractDependencyAddress(b, rootTxRelayer, multiBContRootAddr, multiCContRootAddr,
		multiContSetBAddrFunc, sender)

	// create inputs for contract calls
	singleContInputs := map[string][]byte{
		"calc": getTxInput(b, singleContCalcFunc, []interface{}{big.NewInt(50), big.NewInt(150)}),
		"set":  getTxInput(b, singleContSetFunc, []interface{}{big.NewInt(10)}),
		"get":  getTxInput(b, singleContGetFunc, nil),
	}
	multiContInput := getTxInput(b, multiContFnA, nil)

	// test cases
	testCases := []TxTestCase{
		{
			Name:         "[Child chain] setter 5tx",
			Relayer:      childTxRelayer,
			ContractAddr: singleContChildAddr,
			Input:        [][]byte{singleContInputs["set"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Root chain] setter 5tx",
			Relayer:      rootTxRelayer,
			ContractAddr: singleContRootAddr,
			Input:        [][]byte{singleContInputs["set"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Child chain] getter 5tx",
			Relayer:      childTxRelayer,
			ContractAddr: singleContChildAddr,
			Input:        [][]byte{singleContInputs["get"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Root chain] getter 5tx",
			Relayer:      rootTxRelayer,
			ContractAddr: singleContRootAddr,
			Input:        [][]byte{singleContInputs["get"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Child chain] set,get,calculate 15tx",
			Relayer:      childTxRelayer,
			ContractAddr: singleContChildAddr,
			Input:        [][]byte{singleContInputs["get"], singleContInputs["set"], singleContInputs["calc"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Root chain] set,get,calculate 15tx",
			Relayer:      rootTxRelayer,
			ContractAddr: singleContRootAddr,
			Input:        [][]byte{singleContInputs["get"], singleContInputs["set"], singleContInputs["calc"]},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Child chain] multi contract call 5tx",
			Relayer:      childTxRelayer,
			ContractAddr: multiAContChildAddr,
			Input:        [][]byte{multiContInput},
			Sender:       sender,
			TxNumber:     5,
		},
		{
			Name:         "[Root chain] multi contract call 5tx",
			Relayer:      rootTxRelayer,
			ContractAddr: multiAContRootAddr,
			Input:        [][]byte{multiContInput},
			Sender:       sender,
			TxNumber:     5,
		},
	}

	return testCases, cleanUpFn
}

// TxTestCase represents a test case data to be run with txTestCasesExecutor
type TxTestCase struct {
	Name         string
	Relayer      txrelayer.TxRelayer
	ContractAddr ethgo.Address
	Input        [][]byte
	Sender       ethgo.Key
	TxNumber     int
}

// TxTestCasesExecutor executes transactions from testInput and waits in separate
// go routins for each tx receipt
func TxTestCasesExecutor(b *testing.B, testInput TxTestCase) {
	b.Helper()
	b.Run(testInput.Name, func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		var wg sync.WaitGroup

		// submit all tx 'repeatCall' times
		for i := 0; i < testInput.TxNumber; i++ {
			// call contract for the all inputs
			for j := 0; j < len(testInput.Input); j++ {
				nonce, err := testInput.Relayer.Client().Eth().GetNonce(testInput.Sender.Address(), ethgo.Pending)
				require.NoError(b, err)

				// the tx is submitted to the blockchain without waiting for the receipt,
				// since we want to have multiple tx in one block
				txHash, err := testInput.Relayer.SumbitTransaction(
					&ethgo.Transaction{
						To:    &testInput.ContractAddr,
						Input: testInput.Input[j],
					}, testInput.Sender)
				require.NoError(b, err)
				require.NotEqual(b, ethgo.ZeroHash, txHash)

				wg.Add(1)

				// wait for receipt of submitted tx in a separate routine, and continue with the next tx
				go func(hash ethgo.Hash) {
					defer wg.Done()

					receipt, err := testInput.Relayer.WaitForReceipt(hash)
					require.NoError(b, err)
					require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
				}(txHash)

				// wait for tx to be added in mem pool so that we can create tx with the next nonce
				waitForNextNonce(b, nonce, testInput.Sender.Address(), testInput.Relayer)
			}
		}

		wg.Wait()
	})
}

func waitForNextNonce(b *testing.B, nonce uint64, address ethgo.Address, txRelayer txrelayer.TxRelayer) {
	b.Helper()

	startTime := time.Now().UTC()

	for {
		newNonce, err := txRelayer.Client().Eth().GetNonce(address, ethgo.Pending)
		require.NoError(b, err)

		if newNonce > nonce {
			return
		}

		elapsedTime := time.Since(startTime)
		require.True(b, elapsedTime <= 5*time.Second, "tx not added to the mem poolin 2s")
	}
}
