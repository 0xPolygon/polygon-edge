package loadbot

import (
	"context"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/txpool/proto"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"google.golang.org/grpc"
	any "google.golang.org/protobuf/types/known/anypb"
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
}

type Metrics struct {
	m        sync.Mutex
	Total    uint64        // The total number of transactions processed
	Failed   uint64        // The number of failed transactions
	Duration time.Duration // The execution time of the loadbot
}

func (c *Configuration) createConnections() ([]*grpc.ClientConn, error) {
	var connections []*grpc.ClientConn

	for _, url := range c.RPCURLs {
		conn, err := grpc.Dial(url, grpc.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("failed to connect to server: %v", err)
		}
		connections = append(connections, conn)
	}
	return connections, nil
}

func (c *Configuration) createTransactionObjects() ([]*proto.AddTxnReq, error) {
	var transactions []*proto.AddTxnReq
	var nonces map[types.Address]uint64 = make(map[types.Address]uint64)
	var numberOfAccounts = uint64(len(c.Accounts))

	for _, account := range c.Accounts {
		nonces[account] = 0
	}

	for i := uint64(0); i < c.TxnToSend; i++ {
		from := c.Accounts[i%numberOfAccounts]
		to := c.Accounts[(i+1)%numberOfAccounts]
		nonce := nonces[from]
		txn := &types.Transaction{
			To:       &to,
			Gas:      c.Gas,
			Value:    c.Value,
			GasPrice: c.GasPrice,
			Nonce:    nonce,
			V:        []byte{1}, // it is necessary to encode in rlp
		}

		msg := &proto.AddTxnReq{
			Raw: &any.Any{
				Value: txn.MarshalRLP(),
			},
			// from is not encoded in the rlp
			From: from.String(),
		}

		transactions = append(transactions, msg)

		nonces[from] += 1
	}
	return transactions, nil
}

func (c *Configuration) run(connections []*grpc.ClientConn, txns []*proto.AddTxnReq) *Metrics {
	ticker := time.NewTicker(1 * time.Second / time.Duration(c.TPS))
	defer ticker.Stop()

	connectionID := 0
	numberOfConnection := len(connections)

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
			connection := connections[connectionID%numberOfConnection]
			client := txpoolOp.NewTxnPoolOperatorClient(connection)

			wg.Add(1)
			go func(req *proto.AddTxnReq) {
				defer wg.Done()
				metrics.m.Lock()
				metrics.Total += 1
				metrics.m.Unlock()
				_, err := client.AddTxn(ctx, req)

				if err != nil {
					metrics.m.Lock()
					metrics.Failed += 1
					metrics.m.Unlock()
				}
			}(txns[transactionID])

			transactionID += 1
			connectionID += 1

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

func Execute(configuration *Configuration) (*Metrics, error) {
	connections, err := configuration.createConnections()
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC clients: %v", err)
	}

	transactions, err := configuration.createTransactionObjects()
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction objects: %v", err)
	}

	metrics := configuration.run(connections, transactions)
	return metrics, nil
}
