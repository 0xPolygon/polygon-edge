package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

var (
	oneEth = framework.EthToWei(1)
	signer = crypto.NewEIP155Signer(100)
)

func waitForBlock(t *testing.T, srv *framework.TestServer, expectedBlocks int, index int) int64 {
	t.Helper()

	systemClient := srv.Operator()
	ctx, cancelFn := context.WithCancel(context.Background())
	stream, err := systemClient.Subscribe(ctx, &empty.Empty{})

	if err != nil {
		cancelFn()
		t.Fatalf("Unable to subscribe to blockchain events")
	}

	evnt, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		t.Fatalf("Invalid stream close")
	}

	if err != nil {
		t.Fatalf("Unable to read blockchain event")
	}

	if len(evnt.Added) != expectedBlocks {
		t.Fatalf("Invalid number of blocks added")
	}

	cancelFn()

	return evnt.Added[index].Number
}

type testAccount struct {
	key     *ecdsa.PrivateKey
	address types.Address
	balance *big.Int
}

func generateTestAccounts(t *testing.T, numAccounts int) []*testAccount {
	t.Helper()

	testAccounts := make([]*testAccount, numAccounts)

	for indx := 0; indx < numAccounts; indx++ {
		testAccount := &testAccount{}
		testAccount.key, testAccount.address = tests.GenerateKeyAndAddr(t)
		testAccounts[indx] = testAccount
	}

	return testAccounts
}

func TestTxPool_RecoverableError(t *testing.T) {
	// Test scenario :
	//
	// 1. Send a first valid transaction with gasLimit = block gas limit - 1
	//
	// 2. Send a second transaction with gasLimit = block gas limit / 2. Since there is not enough gas remaining,
	// the transaction will be pushed back to the pending queue so that is can be executed in the next block.
	//
	// 3. Send a third - valid - transaction, both the previous one and this one should be executed.
	//
	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)

	transactions := []*types.Transaction{
		{
			Nonce:    0,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      22000,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
			From:     senderAddress,
		},
		{
			Nonce:    1,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      22000,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
			From:     senderAddress,
		},
		{
			Nonce:    2,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      22000,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
			From:     senderAddress,
		},
	}

	server := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetBlockLimit(2.5 * 21000)
		config.SetDevInterval(2)
		config.Premine(senderAddress, framework.EthToWei(100))
	})[0]

	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	hashes := make([]web3.Hash, 3)

	for i, tx := range transactions {
		signedTx, err := signer.SignTx(tx, senderKey)
		assert.NoError(t, err)

		response, err := operator.AddTxn(context.Background(), &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: signedTx.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		})
		assert.NoError(t, err, "Unable to send transaction, %v", err)

		txHash := web3.Hash(types.StringToHash(response.TxHash))

		// save for later querying
		hashes[i] = txHash
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// wait for the last tx to be included in a block
	receipt, err := tests.WaitForReceipt(ctx, client.Eth(), hashes[2])
	assert.NoError(t, err)
	assert.NotNil(t, receipt)

	// assert balance moved
	balance, err := client.Eth().GetBalance(web3.Address(receiverAddress), web3.Latest)
	assert.NoError(t, err, "failed to retrieve receiver account balance")
	assert.Equal(t, framework.EthToWei(3).String(), balance.String())

	// Query 1st and 2nd txs
	firstTx, err := client.Eth().GetTransactionByHash(hashes[0])
	assert.NoError(t, err)
	assert.NotNil(t, firstTx)

	secondTx, err := client.Eth().GetTransactionByHash(hashes[1])
	assert.NoError(t, err)
	assert.NotNil(t, secondTx)

	// first two are in one block
	assert.Equal(t, firstTx.BlockNumber, secondTx.BlockNumber)

	// last tx is included in next block
	assert.NotEqual(t, secondTx.BlockNumber, receipt.BlockNumber)
}

func TestTxPool_GetPendingTx(t *testing.T) {
	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)
	// Test scenario:
	// The sender account should send multiple transactions to the receiving address
	// and get correct responses when querying the transaction through JSON-RPC

	startingBalance := framework.EthToWei(100)

	server := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(3)
		config.SetBlockLimit(20000000)
		config.Premine(senderAddress, startingBalance)
	})[0]

	operator := server.TxnPoolOperator()
	client := server.JSONRPC()

	// Construct the transaction
	signedTx, err := signer.SignTx(&types.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(0),
		Gas:      framework.DefaultGasLimit - 1,
		To:       &receiverAddress,
		Value:    oneEth,
		V:        big.NewInt(1),
		From:     types.ZeroAddress,
	}, senderKey)
	assert.NoError(t, err, "failed to sign transaction")

	// Add the transaction
	response, err := operator.AddTxn(context.Background(), &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: signedTx.MarshalRLP(),
		},
		From: types.ZeroAddress.String(),
	})
	assert.NoError(t, err, "Unable to send transaction, %v", err)

	txHash := web3.Hash(types.StringToHash(response.TxHash))

	// Grab the pending transaction from the pool
	tx, err := client.Eth().GetTransactionByHash(txHash)
	assert.NoError(t, err, "Unable to get transaction by hash, %v", err)
	assert.NotNil(t, tx)

	// Make sure the specific fields are not filled yet
	assert.Equal(t, uint64(0), tx.TxnIndex)
	assert.Equal(t, uint64(0), tx.BlockNumber)
	assert.Equal(t, web3.ZeroHash, tx.BlockHash)

	// Wait for the transaction to be included into a block
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receipt, err := tests.WaitForReceipt(ctx, client.Eth(), txHash)
	assert.NoError(t, err)
	assert.NotNil(t, receipt)

	assert.Equal(t, tx.TxnIndex, receipt.TransactionIndex)

	// fields should be updated
	tx, err = client.Eth().GetTransactionByHash(txHash)
	assert.NoError(t, err, "Unable to get transaction by hash, %v", err)
	assert.NotNil(t, tx)

	assert.Equal(t, uint64(0), tx.TxnIndex)
	assert.Equal(t, receipt.BlockNumber, tx.BlockNumber)
	assert.Equal(t, receipt.BlockHash, tx.BlockHash)
}
