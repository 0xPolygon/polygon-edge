package e2e

import (
	"context"
	"crypto/ecdsa"
	"io"
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/txpool"
	txpoolOp "github.com/0xPolygon/minimal/txpool/proto"
	"github.com/0xPolygon/minimal/types"
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
		V:        1, // it is necessary to encode in rlp
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

func TestTxpool_ErrNonceTooLow(t *testing.T) {
	// Test scenario:
	// Add tx with nonce 0
	// -> Check if tx has been parsed
	// Add tx with nonce 0
	// -> tx shouldn't be added, since the nonce is too low

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
		value:         oneEth,
		t:             t,
	})

	_, addErr := clt.AddTxn(context.Background(), addReq)
	if addErr != nil {
		t.Fatalf("Unable to add txn, %v", addErr)
	}

	// Wait for the state transition to be executed
	_ = waitForBlock(t, srv, 1, 0)

	// Add the transaction with lower nonce value than what is
	// currently in the world state
	_, addErr = clt.AddTxn(context.Background(), addReq)
	assert.NotNil(t, addErr)
	assert.Contains(t, addErr.Error(), txpool.ErrNonceTooLow.Error())
}

func TestTxpool_ErrInsufficientFunds(t *testing.T) {
	// Test scenario:
	// Add tx with insufficient funds
	// -> Tx should be discarded because of low funds

	// Predefined values
	gasPrice := big.NewInt(10000)

	referenceKey, referenceAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := framework.EthToWei(1)

	devInterval := 5

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(referenceAddr, defaultBalance)
	})
	srv := srvs[0]

	// TxPool client
	clt := srv.TxnPoolOperator()
	toAddress := types.StringToAddress("1")

	// Add the transaction
	addReq := generateReq(generateTxReqParams{
		nonce:         0,
		referenceAddr: referenceAddr,
		referenceKey:  referenceKey,
		toAddress:     toAddress,
		gasPrice:      gasPrice,
		value:         framework.EthToWei(5),
		t:             t,
	})

	// The transaction shouldn't be added because of low funds
	_, addErr := clt.AddTxn(context.Background(), addReq)
	assert.NotNil(t, addErr)
	assert.Contains(t, addErr.Error(), txpool.ErrInsufficientFunds.Error())
}
