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

type Configuration struct {
	TPS       uint64
	Value     *big.Int
	Gas       uint64
	GasPrice  *big.Int
	Accounts  []types.Address
	RPCURLs   []string
	ChainID   uint64
	TxnToSend uint64 // The number of transactions to send
	GRPCUrl   string
}

type Metrics struct {
	m         sync.Mutex
	Total     uint64        // The total number of transactions processed
	Failed    uint64        // The number of failed transactions
	Duration  time.Duration // The execution time of the loadbot
	TxnHashes []web3.Hash
}

func (c *Configuration) createClients() ([]*jsonrpc.Client, error) {
	var clients []*jsonrpc.Client

	for _, url := range c.RPCURLs {
		conn, err := jsonrpc.NewClient(fmt.Sprintf("http://%s", url))
		if err != nil {
			return nil, fmt.Errorf("failed to connect to server: %v", err)
		}
		clients = append(clients, conn)
	}
	return clients, nil
}

func (c *Configuration) createTransactionObjects() ([]*web3.Transaction, error) {
	var transactions []*web3.Transaction
	var nonces = make(map[types.Address]uint64)
	var numberOfAccounts = uint64(len(c.Accounts))

	for _, account := range c.Accounts {
		nonces[account] = 0
	}

	for i := uint64(0); i < c.TxnToSend; i++ {
		from := c.Accounts[i%numberOfAccounts]
		to := c.Accounts[(i+1)%numberOfAccounts]
		nonce := nonces[from]
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
