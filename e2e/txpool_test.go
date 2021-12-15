package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/txpool"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
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
	systemClient := srv.Operator()
	ctx, cancelFn := context.WithCancel(context.Background())
	stream, err := systemClient.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		cancelFn()
		t.Fatalf("Unable to subscribe to blockchain events")
	}

	evnt, err := stream.Recv()
	if err == io.EOF {
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

			_, addErr := clt.AddTxn(context.Background(), addReq)

			if errors.Is(testCase.expectedError, txpool.ErrNonceTooLow) {
				if addErr != nil {
					t.Fatalf("Unable to add txn, %v", addErr)
				}

				// Wait for the state transition to be executed
				_ = waitForBlock(t, srv, 1, 0)

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

	devInterval := 5

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(referenceAddr, defaultBalance)
	})
	srv := srvs[0]
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

	// Add the transactions with the following nonce order
	nonces := []uint64{0, 2}
	for i := 0; i < len(nonces); i++ {
		addReq := generateReq(nonces[i])

		_, addErr := clt.AddTxn(context.Background(), addReq)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}
	}

	// Wait for the state transition to be executed
	_ = waitForBlock(t, srv, 1, 0)

	// Get to account balance
	// Only the first tx should've gone through
	toAccountBalance := framework.GetAccountBalance(toAddress, client, t)
	assert.Equalf(t,
		oneEth.String(),
		toAccountBalance.String(),
		"To address balance mismatch after series of transactions",
	)

	// Add the transaction with the gap nonce value
	addReq := generateReq(1)

	_, addErr := clt.AddTxn(context.Background(), addReq)
	if addErr != nil {
		t.Fatalf("Unable to add txn, %v", addErr)
	}

	// Wait for the state transition to be executed
	_ = waitForBlock(t, srv, 1, 0)

	// Now both the added tx and the shelved tx should've gone through
	toAccountBalance = framework.GetAccountBalance(toAddress, client, t)
	assert.Equalf(t,
		framework.EthToWei(3).String(),
		toAccountBalance.String(),
		"To address balance mismatch after gap transaction",
	)
}

type testAccount struct {
	key     *ecdsa.PrivateKey
	address types.Address
}

func generateTestAccounts(numAccounts int, t *testing.T) []*testAccount {
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

	// Each account should add 40 transactions
	numIterations := 200
	numAccounts := 5

	testAccounts := generateTestAccounts(numAccounts, t)

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetBlockLimit(20000000)
		for _, testAccount := range testAccounts {
			config.Premine(testAccount.address, defaultBalance)
		}
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	// Required default values
	signer := crypto.NewEIP155Signer(100)

	// TxPool client
	clt := srv.TxnPoolOperator()
	toAddress := types.StringToAddress("1")
	defaultValue := framework.EthToWeiPrecise(1, 15) // 0.001 ETH

	generateTx := func(account *testAccount) *types.Transaction {
		nextNonce, err := client.Eth().GetNonce(web3.Address(account.address), web3.Latest)
		if err != nil {
			t.Fatalf("Unable to get nonce")
		}

		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    nextNonce,
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

	var wg sync.WaitGroup

	// Add the transactions with the following nonce order
	for i := 0; i < numAccounts; i++ {
		// Start a concurrent loop
		wg.Add(1)
		go func(index int, clt txpoolOp.TxnPoolOperatorClient, testAccounts []*testAccount) {
			defer wg.Done()

			for innerIndex := 0; innerIndex < numIterations/numAccounts; innerIndex++ {
				txHash, err := client.Eth().SendRawTransaction(generateTx(testAccounts[index]).MarshalRLP())
				if err != nil {
					t.Errorf("Unable to send txn, %v", err)
				}

				waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second*30)
				defer waitCancel()

				_, err = tests.WaitForReceipt(waitCtx, srv.JSONRPC().Eth(), txHash)
				if err != nil {
					t.Errorf("Unable to wait for receipt, %v", err)
					return
				}
			}
		}(i, clt, testAccounts)
	}
	wg.Wait()

	// Make sure the transactions went through
	for _, account := range testAccounts {
		nonce, err := client.Eth().GetNonce(web3.Address(account.address), web3.Latest)
		if err != nil {
			t.Fatalf("Unable to fetch block")
		}
		assert.Equal(t, uint64(numIterations/numAccounts), nonce)
	}
}

func TestPriceLimit(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	_, receiverAddr := tests.GenerateKeyAndAddr(t)

	testCases := []struct {
		name             string
		numNodes         int
		locals           []string
		noLocals         bool
		priceLimit       uint64
		gasPrice         int64
		err              error // JSON-RPC error
		numReceivedNodes int64
	}{
		{
			name:             "tx should propagate",
			numNodes:         3,
			locals:           nil,
			noLocals:         false,
			priceLimit:       10,
			gasPrice:         100,
			err:              nil,
			numReceivedNodes: 3,
		},
		{
			name:             "tx should be rejected due to low gas price",
			numNodes:         3,
			locals:           nil,
			noLocals:         true,
			priceLimit:       10,
			gasPrice:         5,
			err:              errors.New("{\"code\":-32600,\"message\":\"transaction underpriced\"}"),
			numReceivedNodes: 0,
		},
		{
			name:             "tx should not propagate",
			numNodes:         3,
			locals:           nil,
			noLocals:         false,
			priceLimit:       10,
			gasPrice:         0,
			err:              nil,
			numReceivedNodes: 1, // only first node receive
		},
		{
			name:             "tx should propagate because sender address is treated as local transaction",
			numNodes:         3,
			locals:           []string{senderAddr.String()},
			noLocals:         true,
			priceLimit:       10,
			gasPrice:         0,
			err:              nil,
			numReceivedNodes: 3,
		},
	}

	defaultConfig := func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDummy)
		config.Premine(senderAddr, framework.EthToWei(10))
		config.SetSeal(true)
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			srvs := framework.NewTestServers(t, tt.numNodes, func(config *framework.TestServerConfig) {
				defaultConfig(config)
				config.SetLocals(tt.locals)
				config.SetNoLocals(tt.noLocals)
				config.SetPriceLimit(&tt.priceLimit)
			})
			framework.MultiJoinSerial(t, srvs)

			// wait until gossip protocol build mesh network (https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.0.md)
			time.Sleep(time.Second * 2)

			tx, err := signer.SignTx(&types.Transaction{
				Nonce:    0,
				From:     senderAddr,
				To:       &receiverAddr,
				Value:    framework.EthToWei(1),
				Gas:      1000000,
				GasPrice: big.NewInt(tt.gasPrice),
				Input:    []byte{},
			}, senderKey)
			assert.NoError(t, err, "failed to sign transaction")

			_, err = srvs[0].JSONRPC().Eth().SendRawTransaction(tx.MarshalRLP())
			if tt.err != nil {
				assert.Equal(t, tt.err.Error(), err.Error())
			} else {
				assert.NoError(t, err, "failed to send transaction")
			}

			for i, srv := range srvs {
				srv := srv
				shouldHaveTxPool := false
				subTestName := fmt.Sprintf("node %d shouldn't have tx in txpool", i)
				if i < int(tt.numReceivedNodes) {
					shouldHaveTxPool = true
					subTestName = fmt.Sprintf("node %d should have tx in txpool", i)
				}

				t.Run(subTestName, func(t *testing.T) {
					t.Parallel()
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()
					res, err := framework.WaitUntilTxPoolFilled(ctx, srv, 1)

					if shouldHaveTxPool {
						assert.NoError(t, err)
						assert.Equal(t, uint64(1), res.Length)
					} else {
						assert.ErrorIs(t, err, tests.ErrTimeout)
					}
				})
			}
		})
	}
}

func TestInvalidTransactionRecover(t *testing.T) {
	// Test scenario :
	//
	// 1. Send a transaction with gasLimit > block gas limit.
	//		-> The transaction should not be applied, and the nonce should not be incremented.
	//
	// 2. Send a second transaction with a gasLimit < block gas limit.
	//		-> The transaction should be applied, and the receiver balance increased.
	senderKey, senderAddress := tests.GenerateKeyAndAddr(t)
	_, receiverAddress := tests.GenerateKeyAndAddr(t)
	testTable := []struct {
		name              string
		sender            types.Address
		receiver          types.Address
		nonce             uint64
		submittedGasLimit uint64
		value             *big.Int
		shouldSucceed     bool
	}{
		{
			name:              "Invalid transfer caused by exceeding block gas limit",
			sender:            senderAddress,
			receiver:          receiverAddress,
			nonce:             0,
			submittedGasLimit: 5000000000,
			value:             oneEth,
			shouldSucceed:     false,
		},
		{
			name:              "Valid transfer #1",
			sender:            senderAddress,
			receiver:          receiverAddress,
			nonce:             0,
			submittedGasLimit: framework.DefaultGasLimit,
			value:             oneEth,
			shouldSucceed:     true,
		},
	}

	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(5)
		config.Premine(senderAddress, framework.EthToWei(100))
	})

	server := servers[0]
	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	ctx := context.Background()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			tx, err := signer.SignTx(&types.Transaction{
				Nonce:    testCase.nonce,
				GasPrice: big.NewInt(10000),
				Gas:      testCase.submittedGasLimit,
				To:       &testCase.receiver,
				Value:    testCase.value,
				V:        big.NewInt(1),
				From:     testCase.sender,
			}, senderKey)
			assert.NoError(t, err, "failed to sign transaction")

			_, err = operator.AddTxn(ctx, &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: tx.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			})
			assert.NoError(t, err, "failed to add txn using operator")

			_ = waitForBlock(t, server, 1, 0)

			balance, err := client.Eth().GetBalance(web3.Address(receiverAddress), web3.Latest)
			assert.NoError(t, err, "failed to retrieve receiver account balance")

			if testCase.shouldSucceed {
				assert.Equal(t, oneEth.String(), balance.String())
			} else {
				assert.Equal(t, framework.EthToWei(0).String(), balance.String())
			}
		})
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
	testTable := []struct {
		name          string
		sender        types.Address
		senderKey     *ecdsa.PrivateKey
		receiver      types.Address
		nonce         uint64
		gas           uint64
		value         *big.Int
		shouldSucceed bool
	}{
		{
			name:          "Valid transaction #1",
			sender:        senderAddress,
			senderKey:     senderKey,
			receiver:      receiverAddress,
			nonce:         0,
			gas:           framework.DefaultGasLimit - 1,
			value:         oneEth,
			shouldSucceed: true,
		},
		{
			name:          "Invalid transaction (no enough gas remaining in pool)",
			sender:        senderAddress,
			senderKey:     senderKey,
			receiver:      receiverAddress,
			nonce:         1,
			gas:           framework.DefaultGasLimit / 2,
			value:         oneEth,
			shouldSucceed: false,
		},
		{
			name:          "Valid transaction #2",
			sender:        senderAddress,
			senderKey:     senderKey,
			receiver:      receiverAddress,
			nonce:         2,
			gas:           framework.DefaultGasLimit / 2,
			value:         oneEth,
			shouldSucceed: true,
		},
	}

	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(5)
		config.Premine(senderAddress, framework.EthToWei(100))
	})

	server := servers[0]
	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	ctx := context.Background()

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			tx, err := signer.SignTx(&types.Transaction{
				Nonce:    testCase.nonce,
				GasPrice: big.NewInt(10000),
				Gas:      testCase.gas,
				To:       &testCase.receiver,
				Value:    testCase.value,
				V:        big.NewInt(27),
				From:     testCase.sender,
			}, testCase.senderKey)
			assert.NoError(t, err, "failed to sign transaction")

			_, err = operator.AddTxn(ctx, &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: tx.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			})
			assert.NoError(t, err, "failed to add txn using operator")

			if testCase.shouldSucceed {
				_ = waitForBlock(t, server, 1, 0)
			}
		})
	}

	balance, err := client.Eth().GetBalance(web3.Address(receiverAddress), web3.Latest)
	assert.NoError(t, err, "failed to retrieve receiver account balance")

	assert.Equal(t, framework.EthToWei(3).String(), balance.String())

	blockNumber, err := client.Eth().BlockNumber()
	assert.NoError(t, err, "failed to retrieve most recent block number")
	assert.Equal(t, blockNumber, uint64(2))
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
		config.SetDevInterval(5)
		config.SetPriceLimit(&zeroPriceLimit)
		config.SetNoLocals(true)
		config.SetBlockLimit(20000000)
		config.Premine(senderAddress, startingBalance)
	})

	server := servers[0]
	client := server.JSONRPC()
	operator := server.TxnPoolOperator()
	ctx := context.Background()
	var nonce uint64 = 0
	var nonceMux sync.Mutex
	var wg sync.WaitGroup

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
	_ = waitForBlock(t, server, 1, 0)

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

	servers := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(5)
		config.SetBlockLimit(20000000)
		config.Premine(senderAddress, startingBalance)
	})

	server := servers[0]
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
	txHash, err := client.Eth().SendRawTransaction(signedTx.MarshalRLP())
	if err != nil {
		t.Fatalf("Unable to send transaction, %v", err)
	}

	// Grab the pending transaction from the pool
	txn, getErr := client.Eth().GetTransactionByHash(txHash)
	if getErr != nil {
		t.Fatalf("Unable to get transaction by hash, %v", getErr)
	}
	assert.NotNil(t, txn)

	// Make sure the specific fields are not filled yet
	assert.Equal(t, uint64(0), txn.TxnIndex)
	assert.Equal(t, uint64(0), txn.BlockNumber)
	assert.Equal(t, web3.ZeroHash, txn.BlockHash)

	// Wait for the transaction to be included into a block
	blockNum := waitForBlock(t, server, 1, 0)

	block, blockErr := client.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), false)
	if blockErr != nil {
		t.Fatalf("Unable to fetch block, %v", blockErr)
	}

	txn, getErr = client.Eth().GetTransactionByHash(txHash)
	if getErr != nil {
		t.Fatalf("Unable to get transaction by hash, %v", getErr)
	}
	assert.NotNil(t, txn)

	assert.Equal(t, uint64(0), txn.TxnIndex)
	assert.Equal(t, block.Number, txn.BlockNumber)
	assert.Equal(t, block.Hash, txn.BlockHash)
}
