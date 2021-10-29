package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"io"
	"math/big"
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
		V:        []byte{1}, // it is necessary to encode in rlp
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
			V:        []byte{1}, // it is necessary to encode in rlp
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
			numNodes:         5,
			locals:           nil,
			noLocals:         false,
			priceLimit:       10,
			gasPrice:         100,
			err:              nil,
			numReceivedNodes: 5,
		},
		{
			name:             "tx should be rejected due to low gas price",
			numNodes:         5,
			locals:           nil,
			noLocals:         true,
			priceLimit:       10,
			gasPrice:         5,
			err:              errors.New("{\"code\":-32600,\"message\":\"transaction underpriced\"}"),
			numReceivedNodes: 0,
		},
		{
			name:             "tx should not propagate",
			numNodes:         5,
			locals:           nil,
			noLocals:         false,
			priceLimit:       10,
			gasPrice:         0,
			err:              nil,
			numReceivedNodes: 1, // only first node receive
		},
		{
			name:             "tx should propagate because sender address is treated as local transaction",
			numNodes:         5,
			locals:           []string{senderAddr.String()},
			noLocals:         true,
			priceLimit:       10,
			gasPrice:         0,
			err:              nil,
			numReceivedNodes: 5, // only first node receive
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
				V:        []byte{1},
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
				V:        []byte{1},
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
