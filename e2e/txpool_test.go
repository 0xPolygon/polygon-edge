package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"

	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/assert"
)

var (
	oneEth         = framework.EthToWei(1)
	defaultChainID = uint64(100)
	signer         = crypto.NewSigner(chain.AllForksEnabled.At(0), defaultChainID)
)

type generateTxReqParams struct {
	nonce         uint64
	referenceAddr types.Address
	referenceKey  *ecdsa.PrivateKey
	toAddress     types.Address
	gasPrice      *big.Int
	gasFeeCap     *big.Int
	gasTipCap     *big.Int
	value         *big.Int
	t             *testing.T
}

func generateTx(params generateTxReqParams) *types.Transaction {
	var unsignedTx *types.Transaction

	if params.gasPrice != nil {
		unsignedTx = types.NewTx(&types.LegacyTx{
			Nonce: params.nonce,
			To:    &params.toAddress,
			Gas:   1000000,
			Value: params.value,
		})
		unsignedTx.SetGasPrice(params.gasPrice)
	} else {
		unsignedTx = types.NewTx(&types.DynamicFeeTx{
			Nonce:   params.nonce,
			To:      &params.toAddress,
			Gas:     1000000,
			Value:   params.value,
			ChainID: new(big.Int).SetUint64(defaultChainID),
		})
		unsignedTx.SetGasFeeCap(params.gasFeeCap)
		unsignedTx.SetGasTipCap(params.gasTipCap)
	}

	signedTx, err := signer.SignTx(unsignedTx, params.referenceKey)
	require.NoError(params.t, err, "Unable to sign transaction")

	return signedTx
}

func generateReq(params generateTxReqParams) *txpoolOp.AddTxnReq {
	msg := &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: generateTx(params).MarshalRLP(),
		},
		From: types.ZeroAddress.String(),
	}

	return msg
}

func TestTxPool_ErrorCodes(t *testing.T) {
	gasPrice := big.NewInt(1000000000)
	gasFeeCap := big.NewInt(1000000000)
	gasTipCap := big.NewInt(100000000)
	devInterval := 5

	testTable := []struct {
		name           string
		defaultBalance *big.Int
		txValue        *big.Int
		gasPrice       *big.Int
		gasFeeCap      *big.Int
		gasTipCap      *big.Int
		expectedError  error
	}{
		{
			// Test scenario:
			// Add legacy tx with nonce 0
			// -> Check if tx has been parsed
			// Add tx with nonce 0
			// -> tx shouldn't be added, since the nonce is too low
			name:           "ErrNonceTooLow - legacy",
			defaultBalance: framework.EthToWei(10),
			txValue:        oneEth,
			gasPrice:       gasPrice,
			expectedError:  txpool.ErrNonceTooLow,
		},
		{
			// Test scenario:
			// Add dynamic fee tx with nonce 0
			// -> Check if tx has been parsed
			// Add tx with nonce 0
			// -> tx shouldn't be added, since the nonce is too low
			name:           "ErrNonceTooLow - dynamic fees",
			defaultBalance: framework.EthToWei(10),
			txValue:        oneEth,
			gasFeeCap:      gasFeeCap,
			gasTipCap:      gasTipCap,
			expectedError:  txpool.ErrNonceTooLow,
		},
		{
			// Test scenario:
			// Add legacy tx with insufficient funds
			// -> Tx should be discarded because of low funds
			name:           "ErrInsufficientFunds - legacy",
			defaultBalance: framework.EthToWei(1),
			txValue:        framework.EthToWei(5),
			gasPrice:       gasPrice,
			expectedError:  txpool.ErrInsufficientFunds,
		},
		{
			// Test scenario:
			// Add dynamic fee tx with insufficient funds
			// -> Tx should be discarded because of low funds
			name:           "ErrInsufficientFunds - dynamic fee",
			defaultBalance: framework.EthToWei(1),
			txValue:        framework.EthToWei(5),
			gasFeeCap:      gasFeeCap,
			gasTipCap:      gasTipCap,
			expectedError:  txpool.ErrInsufficientFunds,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			referenceKey, referenceAddr := tests.GenerateKeyAndAddr(t)

			// Set up the test server
			srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
				config.SetConsensus(framework.ConsensusDev)
				config.SetDevInterval(devInterval)
				config.Premine(referenceAddr, testCase.defaultBalance)
				config.SetBurnContract(types.StringToAddress("0xBurnContract"))
			})
			srv := srvs[0]

			// TxPool client
			clt := srv.TxnPoolOperator()
			toAddress := types.StringToAddress("1")

			// Add the initial transaction
			addReq := generateReq(generateTxReqParams{
				nonce:         0,
				referenceAddr: referenceAddr,
				referenceKey:  referenceKey,
				toAddress:     toAddress,
				gasPrice:      testCase.gasPrice,
				gasFeeCap:     testCase.gasFeeCap,
				gasTipCap:     testCase.gasTipCap,
				value:         testCase.txValue,
				t:             t,
			})

			addResponse, addErr := clt.AddTxn(context.Background(), addReq)

			if errors.Is(testCase.expectedError, txpool.ErrNonceTooLow) {
				if addErr != nil {
					t.Fatalf("Unable to add txn, %v", addErr)
				}

				// Wait for the state transition to be executed
				receiptCtx, waitCancelFn := context.WithTimeout(
					context.Background(),
					time.Duration(devInterval*2)*time.Second,
				)
				defer waitCancelFn()

				convertedHash := types.StringToHash(addResponse.TxHash)
				_, receiptErr := tests.WaitForReceipt(receiptCtx, srv.JSONRPC().Eth(), ethgo.Hash(convertedHash))

				if receiptErr != nil {
					t.Fatalf("Unable to get receipt, %v", receiptErr)
				}

				// Add the transaction with lower nonce value than what is
				// currently in the world state
				_, addErr = clt.AddTxn(context.Background(), addReq)
			}

			assert.NotNil(t, addErr)
			assert.Contains(t, addErr.Error(), testCase.expectedError.Error())
		})
	}
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
	// the transaction will be pushed back to the pending queue so that can be executed in the next block.
	//
	// 3. Send a third - valid - transaction, both the previous one and this one should be executed.
	//
	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)

	transactions := []*types.Transaction{
		types.NewTx(&types.LegacyTx{
			Nonce:    0,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      22000,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
			From:     senderAddress,
		}),
		types.NewTx(&types.LegacyTx{
			Nonce:    1,
			GasPrice: big.NewInt(framework.DefaultGasPrice),
			Gas:      22000,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
		}),
		types.NewTx(&types.DynamicFeeTx{
			Nonce:     2,
			GasFeeCap: big.NewInt(framework.DefaultGasPrice),
			GasTipCap: big.NewInt(1000000000),
			Gas:       22000,
			To:        &receiverAddress,
			Value:     oneEth,
			ChainID:   new(big.Int).SetUint64(defaultChainID),
			V:         big.NewInt(27),
		}),
	}

	server := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetBlockLimit(2.5 * 21000)
		config.SetDevInterval(2)
		config.Premine(senderAddress, framework.EthToWei(100))
		config.SetBurnContract(types.StringToAddress("0xBurnContract"))
	})[0]

	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	hashes := make([]ethgo.Hash, 3)

	for i, tx := range transactions {
		signedTx, err := signer.SignTx(tx, senderKey)
		assert.NoError(t, err)

		response, err := operator.AddTxn(context.Background(), &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: signedTx.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		})
		require.NoError(t, err, "Unable to send transaction, %v", err)

		txHash := ethgo.Hash(types.StringToHash(response.TxHash))

		// save for later querying
		hashes[i] = txHash
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	// wait for the last tx to be included in a block
	receipt, err := tests.WaitForReceipt(ctx, client.Eth(), hashes[2])
	require.NoError(t, err)
	require.NotNil(t, receipt)

	// assert balance moved
	balance, err := client.Eth().GetBalance(ethgo.Address(receiverAddress), ethgo.Latest)
	require.NoError(t, err, "failed to retrieve receiver account balance")
	require.Equal(t, framework.EthToWei(3).String(), balance.String())

	// Query 1st and 2nd txs
	firstTx, err := client.Eth().GetTransactionByHash(hashes[0])
	require.NoError(t, err)
	require.NotNil(t, firstTx)

	secondTx, err := client.Eth().GetTransactionByHash(hashes[1])
	require.NoError(t, err)
	require.NotNil(t, secondTx)

	// first two are in one block
	require.Equal(t, firstTx.BlockNumber, secondTx.BlockNumber)

	// last tx is included in next block
	require.NotEqual(t, secondTx.BlockNumber, receipt.BlockNumber)
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
		config.SetDevInterval(3)
		config.SetBlockLimit(20000000)
		config.Premine(senderAddress, startingBalance)
		config.SetBurnContract(types.StringToAddress("0xBurnContract"))
	})[0]

	operator := server.TxnPoolOperator()
	client := server.JSONRPC()

	signedTx, err := signer.SignTx(types.NewTx(&types.LegacyTx{
		Nonce:    0,
		GasPrice: big.NewInt(1000000000),
		Gas:      framework.DefaultGasLimit - 1,
		To:       &receiverAddress,
		Value:    oneEth,
		V:        big.NewInt(1),
		From:     types.ZeroAddress,
	}), senderKey)
	assert.NoError(t, err, "failed to sign transaction")

	// Add the transaction
	response, err := operator.AddTxn(context.Background(), &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: signedTx.MarshalRLP(),
		},
		From: types.ZeroAddress.String(),
	})
	assert.NoError(t, err, "Unable to send transaction, %v", err)

	txHash := ethgo.Hash(types.StringToHash(response.TxHash))

	// Grab the pending transaction from the pool
	tx, err := client.Eth().GetTransactionByHash(txHash)
	assert.NoError(t, err, "Unable to get transaction by hash, %v", err)
	assert.NotNil(t, tx)

	// Make sure the specific fields are not filled yet
	assert.Equal(t, uint64(0), tx.TxnIndex)
	assert.Equal(t, uint64(0), tx.BlockNumber)
	assert.Equal(t, ethgo.ZeroHash, tx.BlockHash)

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
