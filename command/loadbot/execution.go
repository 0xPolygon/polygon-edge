package loadbot

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	txPoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	"google.golang.org/grpc"
	"math/big"
	"sync"
	"time"
)

type Configuration struct {
	TPS           uint64
	AccountsCount uint64
	Value         int64
	Count         uint64
	JSONRPCs      []string
	GRPCs         []string
	Sponsor       Account
}

type Metrics struct {
	m                          sync.Mutex
	Duration                   time.Duration
	TotalTransactionsSentCount uint64
	FailedTransactionsCount    uint64
}

type Account struct {
	Address    types.Address
	PrivateKey ecdsa.PrivateKey
}

// generateRandomValue creates a random value used in a transaction.
// The max value that can be generated represents 0.01 ETH.
func generateRandomValue() (*big.Int, error) {
	b, err := rand.Int(rand.Reader, big.NewInt(10000000000000000))
	if err != nil {
		return nil, fmt.Errorf("failed to create random number: %v", err)
	}
	return b, nil
}

func createJsonRpcClient(endpoint string) (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create new JSON RPC client: %v", err)
	}
	return client, nil
}

func createJsonRpcClients(endpoints []string) ([]*jsonrpc.Client, error) {
	var clients []*jsonrpc.Client

	for i := 0; i < len(endpoints); i++ {
		client, err := jsonrpc.NewClient(endpoints[i])
		if err != nil {
			return nil, fmt.Errorf("failed to create JSON-RPC client: %v", err)
		}

		clients = append(clients, client)
	}

	return clients, nil
}

func createGRpcClient(endpoint string) (*txPoolOp.TxnPoolOperatorClient, error) {
	conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %v", err)
	}

	client := txPoolOp.NewTxnPoolOperatorClient(conn)
	return &client, nil
}

func createGRpcClients(endpoints []string) ([]*txPoolOp.TxnPoolOperatorClient, error) {
	var clients []*txPoolOp.TxnPoolOperatorClient

	for _, endpoint := range endpoints {
		client, err := createGRpcClient(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC client: %v", err)
		}
		clients = append(clients, client)
	}
	return clients, nil
}

func generateAccounts(n uint64) ([]*Account, error) {
	var accounts []*Account

	for i := uint64(0); i < n; i++ {
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("failed to create ecdsa key pair: %v", err)
		}

		account := Account{
			Address:    crypto.PubKeyToAddress(&privateKey.PublicKey),
			PrivateKey: *privateKey,
		}

		accounts = append(accounts, &account)
	}
	return accounts, nil
}

func verifySponsorBalance(endpoint string, sponsor *Account, n uint64) error {
	client, err := createJsonRpcClient(endpoint)
	if err != nil {
		return fmt.Errorf("failed to create JSON-RPC client: %v", err)
	}

	// Verify if the sponsor has enough funds
	balance, err := client.Eth().GetBalance(web3.Address(sponsor.Address), -1)
	if err != nil {
		return fmt.Errorf("failed to get sponsor balance: %v", err)
	}

	required := big.NewInt(int64(n))
	required = required.Mul(required, framework.EthToWei(1000))

	if balance.Cmp(required) == -1 {
		return fmt.Errorf("not enough balance to prefund accounts: %v", err)
	}
	return nil
}

// prefundAccounts uses the sponsor account to prefund each account with 1000 ETH.
func prefundAccounts(endpoint string, accounts []*Account, sponsor *Account) error {
	nonce := uint64(0)

	signer := crypto.NewEIP155Signer(100)

	client, err := createJsonRpcClient(endpoint)
	if err != nil {
		return fmt.Errorf("failed to create JSON-RPC client: %v", err)
	}

	for _, account := range accounts {
		txn, err := signer.SignTx(&types.Transaction{
			From:     sponsor.Address,
			To:       &account.Address,
			Gas:      1000000,
			Value:    framework.EthToWei(1000),
			GasPrice: big.NewInt(0x100000),
			Nonce:    nonce,
			V:        []byte{1}, // it is necessary to encode in rlp
		}, &sponsor.PrivateKey)
		if err != nil {
			return fmt.Errorf("failed to sign transaction during prefund phase: %v", err)
		}

		_, err = client.Eth().SendRawTransaction(txn.MarshalRLP())
		if err != nil {
			return fmt.Errorf("failed to send transaction during prefund phase: %v", err)
		}

		nonce += 1
	}
	return nil
}

func waitPrefundEnd(txPoolEndpoint string) error {
	fmt.Println("Waiting for TxPool to be empty to proceed to prefund verification")
	conn, err := grpc.Dial(txPoolEndpoint, grpc.WithInsecure())
	if err != nil {
		return fmt.Errorf("failed to connect to TxPool: %v", err)
	}

	client := txPoolOp.NewTxnPoolOperatorClient(conn)
	_, err = tests.WaitUntilTxPoolEmpty(context.Background(), client)
	if err != nil {
		return fmt.Errorf("error occured while waiting for TxPool to be empty")
	}
	return nil
}

func verifyPrefund(endpoint string, accounts []*Account) error {
	client, err := createJsonRpcClient(endpoint)
	if err != nil {
		return fmt.Errorf("failed to create JSON-RPC client: %v", err)
	}

	block, err := client.Eth().GetBlockByNumber(-1, true)
	if err != nil {
		return fmt.Errorf("failed to get block before verifiying prefund: %v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:

			newBlock, err := client.Eth().GetBlockByNumber(-1, true)
			if err != nil {
				return fmt.Errorf("failed to get block before verifiying prefund: %v", err)
			}

			if newBlock.Number < block.Number+5 {
				continue
			}
			block = newBlock

			for _, account := range accounts {
				balance, err := client.Eth().GetBalance(web3.Address(account.Address), -1)
				if err != nil {
					return fmt.Errorf("failed to retrieve account's balance: %v", err)

				}

				required := framework.EthToWei(1000)

				if balance.Cmp(required) == -1 {
					return fmt.Errorf("account does not have been prefunded correctly")
				}
			}
			return nil
		}
	}
}

func execute(client *jsonrpc.Client, sender Account, receiver Account, value int64) error {
	// Get nonce for the sender account
	nonce, err := client.Eth().GetNonce(web3.Address(sender.Address), -1)
	if err != nil {
		return fmt.Errorf("failed to get sender nonce: %v", err)
	}

	// If required, generate new value for the transaction
	txnValue := big.NewInt(value)
	if value == -1 {
		txnValue, err = generateRandomValue()
		if err != nil {
			return fmt.Errorf("failed to generate new transaction value: %v", err)
		}
	}

	// Create and sign the transaction object
	signer := crypto.NewEIP155Signer(100)

	txn, err := signer.SignTx(&types.Transaction{
		From:     sender.Address,
		To:       &receiver.Address,
		Gas:      1000000,
		Value:    txnValue,
		GasPrice: big.NewInt(0x100000),
		Nonce:    nonce,
		V:        []byte{1}, // it is necessary to encode in rlp
	}, &sender.PrivateKey)

	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	// Send the transaction object
	_, err = client.Eth().SendRawTransaction(txn.MarshalRLP())
	if err != nil {
		return fmt.Errorf("failed to send raw transaction: %v", err)
	}
	return nil
}

func waitForAllTxPool(endpoints []string) {
	// TODO: Handle error
	clients, _ := createGRpcClients(endpoints)

	for _, client := range clients {
		// TODO: Handle error
		status, _ := tests.WaitUntilTxPoolEmpty(context.Background(), *client)
		for status.Length != 0 {
			// TODO: Handle error
			status, _ = tests.WaitUntilTxPoolEmpty(context.Background(), *client)
		}
	}
}

func shutdownAllClients(clients []*jsonrpc.Client) {
	for _, client := range clients {
		// TODO: Handle error
		client.Close()
	}
}

func Run(conf *Configuration, metrics *Metrics) error {
	// Create the ticker
	ticker := time.NewTicker(1 * time.Second / time.Duration(conf.TPS))
	defer ticker.Stop()

	// Record execution time
	start := time.Now()
	defer func() {
		metrics.Duration = time.Since(start)
	}()

	// Generate accounts
	accounts, err := generateAccounts(conf.AccountsCount)
	if err != nil {
		return fmt.Errorf("failed to create accounts: %v", err)
	}

	// Verify if the sponsor has enough funds
	err = verifySponsorBalance(conf.JSONRPCs[0], &conf.Sponsor, conf.AccountsCount)
	if err != nil {
		return fmt.Errorf("failed to verify sponsor's balance: %v", err)
	}

	// Prefund accounts if required
	err = prefundAccounts(conf.JSONRPCs[0], accounts, &conf.Sponsor)
	if err != nil {
		return fmt.Errorf("failed to prefund accounts: %v", err)
	}

	// Wait for prefund to finish
	err = waitPrefundEnd(conf.GRPCs[0])
	if err != nil {
		return fmt.Errorf("failed to wait for prefund process to end: %v", err)
	}

	// Verify prefund
	err = verifyPrefund(conf.JSONRPCs[0], accounts)
	if err != nil {
		return fmt.Errorf("failed to verify prefund: %v", err)
	}

	// Create clients
	clientID := 0
	maxClientID := len(conf.JSONRPCs)
	clients, err := createJsonRpcClients(conf.JSONRPCs)
	if err != nil {
		return fmt.Errorf("failed to create JSON-RPC clients: %v", err)
	}

	// Accounts ID
	accountID := 0
	maxAccountID := len(accounts)

	// Wait for all TxPool to be empty at the end
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
	}()
	defer waitForAllTxPool(conf.GRPCs)

	// Shutdown all clients
	defer shutdownAllClients(clients)

	// Loop and send a transaction at each tick
	for {
		select {
		case <-ticker.C:
			// Register new operation in the metrics
			metrics.m.Lock()
			metrics.TotalTransactionsSentCount += 1
			if metrics.TotalTransactionsSentCount == conf.Count {
				return nil
			}
			metrics.m.Unlock()

			// Select client, sender and receiver
			client := clients[clientID%maxClientID]
			sender := accounts[accountID%maxAccountID]
			receiver := accounts[(accountID+1)%maxAccountID]

			// Update indices
			clientID += 1
			accountID += 1

			// Send the transaction
			wg.Add(1)
			go func(s Account, r Account) {
				wg.Done()
				err := execute(client, s, r, conf.Value)

				// Register an error in the metrics
				if err != nil {
					metrics.m.Lock()
					metrics.FailedTransactionsCount += 1
					metrics.m.Unlock()
				}
			}(*sender, *receiver)
		}
	}
}
