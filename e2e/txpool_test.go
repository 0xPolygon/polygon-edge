package e2e

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"io"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/txpool"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/assert"
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
