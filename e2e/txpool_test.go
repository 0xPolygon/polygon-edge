package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/txpool"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

var (
	oneEth = framework.EthToWei(1)
	signer = crypto.NewEIP155Signer(100)
)

type generateTxReqParams struct {
	nonce         uint64
	referenceAddr types.Address
	referenceKey  *ecdsa.PrivateKey
	toAddress     types.Address
	gasPrice      *big.Int
	value         *big.Int
	t             *testing.T
}

func generateTx(params generateTxReqParams) *types.Transaction {
	signedTx, signErr := signer.SignTx(&types.Transaction{
		Nonce:    params.nonce,
		From:     params.referenceAddr,
		To:       &params.toAddress,
		GasPrice: params.gasPrice,
		Gas:      1000000,
		Value:    params.value,
		V:        big.NewInt(27), // it is necessary to encode in rlp
	}, params.referenceKey)

	if signErr != nil {
		params.t.Fatalf("Unable to sign transaction, %v", signErr)
	}

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
	gasPrice := big.NewInt(10000)
	devInterval := 5

	testTable := []struct {
		name           string
		defaultBalance *big.Int
		txValue        *big.Int
		expectedError  error
	}{
		{
			// Test scenario:
			// Add tx with nonce 0
			// -> Check if tx has been parsed
			// Add tx with nonce 0
			// -> tx shouldn't be added, since the nonce is too low
			"ErrNonceTooLow",
			framework.EthToWei(10),
			oneEth,
			txpool.ErrNonceTooLow,
		},
		{
			// Test scenario:
			// Add tx with insufficient funds
			// -> Tx should be discarded because of low funds
			"ErrInsufficientFunds",
			framework.EthToWei(1),
			framework.EthToWei(5),
			txpool.ErrInsufficientFunds,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			referenceKey, referenceAddr := tests.GenerateKeyAndAddr(t)

			// Set up the test server
			srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
				config.SetConsensus(framework.ConsensusDev)
				config.SetSeal(true)
				config.SetDevInterval(devInterval)
				config.Premine(referenceAddr, testCase.defaultBalance)
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
				gasPrice:      gasPrice,
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
				_, receiptErr := tests.WaitForReceipt(receiptCtx, srv.JSONRPC().Eth(), web3.Hash(convertedHash))
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

func TestTxPool_TransactionCoalescing(t *testing.T) {
	// Test scenario:
	// Add tx with nonce 0
	// -> Check if tx has been parsed
	// Add tx with nonce 2
	// -> tx shouldn't be executed, but shelved for later
	// Add tx with nonce 1
	// -> check if both tx with nonce 1 and tx with nonce 2 are parsed
	// Predefined values
	gasPrice := big.NewInt(10000)

	referenceKey, referenceAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := framework.EthToWei(10)

	// Set up the test server
	ibftManager := framework.NewIBFTServersManager(
		t,
		1,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.SetSeal(true)
			config.Premine(referenceAddr, defaultBalance)
			config.SetBlockTime(1)
		},
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	client := srv.JSONRPC()

	// Required default values
	signer := crypto.NewEIP155Signer(100)

	// TxPool client
	clt := srv.TxnPoolOperator()
	toAddress := types.StringToAddress("1")
	oneEth := framework.EthToWei(1)

	generateTx := func(nonce uint64) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    nonce,
			From:     referenceAddr,
			To:       &toAddress,
			GasPrice: gasPrice,
			Gas:      1000000,
			Value:    oneEth,
			V:        big.NewInt(1), // it is necessary to encode in rlp
		}, referenceKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		return signedTx
	}

	generateReq := func(nonce uint64) *txpoolOp.AddTxnReq {
		msg := &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: generateTx(nonce).MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		}

		return msg
	}

	// testTransaction is a helper structure for
	// keeping track of test transaction execution
	type testTransaction struct {
		txHash web3.Hash // the transaction hash
		block  *uint64   // the block the transaction was included in
	}

	testTransactions := make([]*testTransaction, 0)

	// Add the transactions with the following nonce order
	nonces := []uint64{0, 2}
	for i := 0; i < len(nonces); i++ {
		addReq := generateReq(nonces[i])

		addCtx, addCtxCn := context.WithTimeout(context.Background(), time.Second*10)

		addResp, addErr := clt.AddTxn(addCtx, addReq)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}

		testTransactions = append(testTransactions, &testTransaction{
			txHash: web3.HexToHash(addResp.TxHash),
		})

		addCtxCn()
	}

	// Wait for the first transaction to go through
	ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()

	receipt, receiptErr := tests.WaitForReceipt(ctx, client.Eth(), testTransactions[0].txHash)
	if receiptErr != nil {
		t.Fatalf("unable to wait for receipt, %v", receiptErr)
	}

	testTransactions[0].block = &receipt.BlockNumber

	// Get to account balance
	// Only the first tx should've gone through
	toAccountBalance := framework.GetAccountBalance(t, toAddress, client)
	assert.Equalf(t,
		oneEth.String(),
		toAccountBalance.String(),
		"To address balance mismatch after series of transactions",
	)

	// Add the transaction with the gap nonce value
	addReq := generateReq(1)

	addCtx, addCtxCn := context.WithTimeout(context.Background(), time.Second*10)
	defer addCtxCn()

	addResp, addErr := clt.AddTxn(addCtx, addReq)
	if addErr != nil {
		t.Fatalf("Unable to add txn, %v", addErr)
	}

	testTransactions = append(testTransactions, &testTransaction{
		txHash: web3.HexToHash(addResp.TxHash),
	})

	// Start from 1 since there was previously a txn with nonce 0
	for i := 1; i < len(testTransactions); i++ {
		// Wait for the first transaction to go through
		ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)

		receipt, receiptErr := tests.WaitForReceipt(ctx, client.Eth(), testTransactions[i].txHash)
		if receiptErr != nil {
			t.Fatalf("unable to wait for receipt, %v", receiptErr)
		}

		testTransactions[i].block = &receipt.BlockNumber

		cancelFn()
	}

	// Now both the added tx and the shelved tx should've gone through
	toAccountBalance = framework.GetAccountBalance(t, toAddress, client)
	assert.Equalf(t,
		framework.EthToWei(3).String(),
		toAccountBalance.String(),
		"To address balance mismatch after gap transaction",
	)

	// Make sure the first transaction and the last transaction didn't get included in the same block
	assert.NotEqual(t, *(testTransactions[0].block), *(testTransactions[2].block))
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

func TestTxPool_StressAddition(t *testing.T) {
	// Test scenario:
	// Add a large number of txns to the txpool concurrently
	// Predefined values
	defaultBalance := framework.EthToWei(10000)

	// Each account should add 50 transactions
	numAccounts := 10
	numTxPerAccount := 50

	testAccounts := generateTestAccounts(t, numAccounts)

	// Set up the test server
	srv := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetBlockLimit(20000000)
		for _, testAccount := range testAccounts {
			config.Premine(testAccount.address, defaultBalance)
		}
	})[0]
	client := srv.JSONRPC()

	// Required default values
	signer := crypto.NewEIP155Signer(100)

	// TxPool client
	toAddress := types.StringToAddress("1")
	defaultValue := framework.EthToWeiPrecise(1, 15) // 0.001 ETH

	generateTx := func(account *testAccount, nonce uint64) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    nonce,
			From:     account.address,
			To:       &toAddress,
			GasPrice: big.NewInt(10),
			Gas:      framework.DefaultGasLimit,
			Value:    defaultValue,
			V:        big.NewInt(27), // it is necessary to encode in rlp
		}, account.key)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		return signedTx
	}

	// Spawn numAccounts threads to act as sender workers that will send transactions.
	// The sender worker forwards the transaction hash to the receipt worker.
	// The numAccounts receipt worker threads wait for tx hashes to arrive and wait for their receipts

	var (
		wg           sync.WaitGroup
		errorsLock   sync.Mutex
		workerErrors = make([]error, 0)
	)

	wg.Add(numAccounts)

	appendError := func(err error) {
		errorsLock.Lock()
		defer errorsLock.Unlock()

		workerErrors = append(workerErrors, err)
	}

	sendWorker := func(account *testAccount, receiptsChan chan web3.Hash) {
		defer close(receiptsChan)

		nonce := uint64(0)

		for i := 0; i < numTxPerAccount; i++ {
			tx := generateTx(account, nonce)

			txHash, err := client.Eth().SendRawTransaction(tx.MarshalRLP())
			if err != nil {
				appendError(fmt.Errorf("unable to send txn, %w", err))

				return
			}

			receiptsChan <- txHash

			nonce++
		}
	}

	receiptWorker := func(receiptsChan chan web3.Hash) {
		defer wg.Done()

		for txHash := range receiptsChan {
			waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second*30)

			if _, err := tests.WaitForReceipt(waitCtx, srv.JSONRPC().Eth(), txHash); err != nil {
				appendError(fmt.Errorf("unable to wait for receipt, %w", err))
				waitCancel()

				return
			}

			waitCancel()
		}
	}

	for _, testAccount := range testAccounts {
		receiptsCh := make(chan web3.Hash, numTxPerAccount)
		go sendWorker(
			testAccount,
			receiptsCh,
		)

		go receiptWorker(receiptsCh)
	}

	wg.Wait()

	if len(workerErrors) != 0 {
		t.Fatalf("%v", workerErrors)
	}

	// Make sure the transactions went through
	for _, account := range testAccounts {
		nonce, err := client.Eth().GetNonce(web3.Address(account.address), web3.Latest)
		if err != nil {
			t.Fatalf("Unable to fetch block")
		}

		assert.Equal(t, uint64(numTxPerAccount), nonce)
	}
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

func TestTxPool_ZeroPriceDev(t *testing.T) {
	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)
	// Test scenario:
	// The sender account should send funds to the receiver account.
	// Each transaction should have a gas price set do 0 and be treated
	// as a non-local transaction

	var zeroPriceLimit uint64 = 0

	startingBalance := framework.EthToWei(100)

	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetPriceLimit(&zeroPriceLimit)
		config.SetBlockLimit(20000000)
		config.Premine(senderAddress, startingBalance)
	})

	server := servers[0]
	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	ctx := context.Background()

	var (
		nonce    uint64 = 0
		nonceMux sync.Mutex
		wg       sync.WaitGroup
	)

	sendTx := func() {
		nonceMux.Lock()
		tx, err := signer.SignTx(&types.Transaction{
			Nonce:    nonce,
			GasPrice: big.NewInt(0),
			Gas:      framework.DefaultGasLimit - 1,
			To:       &receiverAddress,
			Value:    oneEth,
			V:        big.NewInt(27),
			From:     types.ZeroAddress,
		}, senderKey)
		assert.NoError(t, err, "failed to sign transaction")

		_, err = operator.AddTxn(ctx, &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: tx.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		})
		assert.NoError(t, err, "failed to add txn using operator")

		nonce++
		nonceMux.Unlock()

		wg.Done()
	}

	numIterations := 100
	numIterationsBig := big.NewInt(int64(numIterations))

	for i := 0; i < numIterations; i++ {
		wg.Add(1)

		go sendTx()
	}

	wg.Wait()

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer waitCancel()

	_, err := tests.WaitForNonce(waitCtx, client.Eth(), web3.BytesToAddress(senderAddress.Bytes()), nonce)
	assert.NoError(t, err)

	receiverBalance, err := client.Eth().GetBalance(web3.Address(receiverAddress), web3.Latest)
	assert.NoError(t, err, "failed to retrieve receiver account balance")

	sentFunds := big.NewInt(0).Mul(numIterationsBig, oneEth)
	assert.Equal(t, sentFunds.String(), receiverBalance.String())

	senderBalance, err := client.Eth().GetBalance(web3.Address(senderAddress), web3.Latest)
	assert.NoError(t, err, "failed to retrieve sender account balance")

	assert.Equal(t, big.NewInt(0).Sub(startingBalance, sentFunds).String(), senderBalance.String())
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
