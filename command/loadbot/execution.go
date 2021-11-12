package loadbot

import (
	"context"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	"google.golang.org/grpc"
	empty "google.golang.org/protobuf/types/known/emptypb"
	"math/big"
	"sync"
	"time"
)

// Configuration represents the loadbot run configuration.
// It contains the required parameters to run the stress test.
type Configuration struct {
	TPS       uint64          // Number of transactions per second
	Value     *big.Int        // The value sent in each transaction
	Gas       uint64          // The transaction's gas
	GasPrice  *big.Int        // The transaction's gas price
	Accounts  []types.Address // All the accounts used by the bot to send transaction
	RPCURLs   []string        // The JSON RPC endpoints, used to send transactions
	ChainID   uint64
	TxnToSend uint64 // The number of transaction to send
	GRPCUrl   string // The gRPC url, used while verifying transactions to check if the TxPool is empty
}

type Metrics struct {
	m         sync.Mutex
	Total     uint64        // The total number of transactions processed
	Failed    uint64        // The number of failed transactions
	Duration  time.Duration // The execution time of the loadbot
	TxnHashes []web3.Hash   // The hashes of the transactions sent
}

// createClients will create JSON RPC clients using the provided addresses in the CLI.
// Each one of these clients will send transaction(s), each one after another.
func (c *Configuration) createClients() ([]*jsonrpc.Client, error) {
	var clients []*jsonrpc.Client

	for _, url := range c.RPCURLs {
		conn, err := jsonrpc.NewClient(url)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to server: %v", err)
		}
		clients = append(clients, conn)
	}
	return clients, nil
}

// We create the transactions using createTransactionObjects before the loadbot send them.
func (c *Configuration) createTransactionObjects() ([]*web3.Transaction, error) {
	var transactions []*web3.Transaction
	var nonces = make(map[types.Address]uint64)
	var numberOfAccounts = uint64(len(c.Accounts))

	// Each loop create one transaction.
	for i := uint64(0); i < c.TxnToSend; i++ {
		// Get sender and receiver.
		from := c.Accounts[i%numberOfAccounts]
		to := c.Accounts[(i+1)%numberOfAccounts]

		// Get sender nonce.
		nonce := nonces[from]

		// Create the transaction object.
		txn := &web3.Transaction{
			From:     web3.Address(from),
			To:       (*web3.Address)(&to),
			Gas:      c.Gas,
			Value:    c.Value,
			GasPrice: c.GasPrice.Uint64(),
			Nonce:    nonce,
			V:        []byte{1}, // it is necessary to encode in rlp
		}

		transactions = append(transactions, txn)

		nonces[from] += 1
	}
	return transactions, nil
}

// run is the main method of the loadbot.
// The TPS is used to determine the rate at which every transaction is sent.
func (c *Configuration) run(clients []*jsonrpc.Client, txns []*web3.Transaction) *Metrics {
	ticker := time.NewTicker(1 * time.Second / time.Duration(c.TPS))
	defer ticker.Stop()

	clientID := 0
	numberOfClients := len(clients)

	transactionID := 0
	numberOfTransactions := len(txns)

	var wg sync.WaitGroup
	ctx := context.Background()

	start := time.Now()
	metrics := Metrics{
		m:        sync.Mutex{},
		Total:    0,
		Failed:   0,
		Duration: 0,
	}
	defer func() {
		metrics.Duration = time.Since(start)
	}()

	fmt.Println("Loadbot started !")
	for {
		select {
		case <-ticker.C:
			client := clients[clientID%numberOfClients]

			wg.Add(1)
			go func(txn *web3.Transaction) {
				defer wg.Done()
				metrics.m.Lock()
				metrics.Total += 1
				metrics.m.Unlock()
				hash, err := client.Eth().SendTransaction(txn)

				if err != nil {
					metrics.m.Lock()
					metrics.Failed += 1
					metrics.m.Unlock()
				}

				metrics.m.Lock()
				metrics.TxnHashes = append(metrics.TxnHashes, hash)
				metrics.m.Unlock()

			}(txns[transactionID])

			transactionID += 1
			clientID += 1

			if transactionID == numberOfTransactions {
				wg.Wait()
				return &metrics
			}

		case <-ctx.Done():
			wg.Wait()
			return &metrics
		}
	}
}

func waitUntilTxPoolEmpty(ctx context.Context, client txpoolOp.TxnPoolOperatorClient) (*txpoolOp.TxnPoolStatusResp, error) {
	res, err := tests.RetryUntilTimeout(ctx, func() (interface{}, bool) {
		subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, _ := client.Status(subCtx, &empty.Empty{})
		if res != nil && res.Length == 0 {
			return res, false
		}
		fmt.Printf("TxPool not empty, %d transactions remaining..\n", res.Length)
		return nil, true
	})

	if err != nil {
		return nil, err
	}
	return res.(*txpoolOp.TxnPoolStatusResp), nil
}

// verifyTxns checks whether a transaction has been properly written to the blockchain.
// First, it waits for the TxPool to be empty.
func (m *Metrics) verifyTxns(jClient *jsonrpc.Client, url string) error {
	conn, err := grpc.Dial(url, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to connect to TxPool: %v", err)
	}
	gClient := txpoolOp.NewTxnPoolOperatorClient(conn)

	_, err = waitUntilTxPoolEmpty(context.Background(), gClient)
	if err != nil {
		return fmt.Errorf("failed to wait until TxPool is empty: %v", err)
	}

	for _, hash := range m.TxnHashes {
		transaction, err := jClient.Eth().GetTransactionByHash(hash)
		if err != nil {
			return fmt.Errorf("failed to retrieve transaction: %v", err)
		}

		if transaction == nil {
			m.Failed += 1
		}
	}
	return nil
}

// Execute creates the JSON RPC clients, the transactions objects, send each one of them and verify the result.
func Execute(configuration *Configuration) (*Metrics, error) {
	clients, err := configuration.createClients()
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC clients: %v", err)
	}

	transactions, err := configuration.createTransactionObjects()
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction objects: %v", err)
	}

	metrics := configuration.run(clients, transactions)

	err = metrics.verifyTxns(clients[0], configuration.GRPCUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to verify txns: %v", err)
	}
	return metrics, nil
}
