package benchmark

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/e2e-polybft/framework"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

var (
	singleContCalcFunc   = abi.MustNewMethod("function compute(uint256 x, uint256 y) public returns (uint256)")
	singleContGetFunc    = abi.MustNewMethod("function getValue() public returns (uint256[] memory)")
	singleContSetFunc    = abi.MustNewMethod("function addValue(uint256 value) public")
	multiContSetAddrFunc = abi.MustNewMethod("function setContractAddr(address _contract) public")
	multiContFnA         = abi.MustNewMethod("function fnA() public returns (uint256)")
)

// The rootChildSendTx function executes test cases that measure transaction execution on both the root and child chains
// To do this, it first calls RootChildSendTxSetUp to set up the testing environment,
// which may include starting the cluster, deploying contracts, and building the test cases.
// After building the test cases, rootChildSendTx returns them along with a cleanup function that should be called
// after the test cases have been executed. The test cases are executed by the TxTestCasesExecutor.
func rootChildSendTx(b *testing.B) {
	b.Helper()
	// set up environment, get test cases and clean up fn
	testCases, cleanUpFn := RootChildSendTxSetUp(b, "", "", "", true)
	defer cleanUpFn()

	// Loop over the test cases and measure the execution time of the transactions
	for _, testInput := range testCases {
		TxTestCasesExecutor(b, testInput)
	}
}

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
		multiContSetAddrFunc, sender)
	setContractDependencyAddress(b, rootTxRelayer, multiAContRootAddr, multiBContRootAddr,
		multiContSetAddrFunc, sender)
	// set C contract address in B contract
	setContractDependencyAddress(b, childTxRelayer, multiBContChildAddr, multiCContChildAddr,
		multiContSetAddrFunc, sender)
	setContractDependencyAddress(b, rootTxRelayer, multiBContRootAddr, multiCContRootAddr,
		multiContSetAddrFunc, sender)

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
