package loadbot

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
	"math/big"
	"sync"
	"sync/atomic"
	"time"
)

type Configuration struct {
	TPS      uint64
	Sender   types.Address
	Receiver types.Address
	Value    *big.Int
	Count    uint64
	JSONRPC  string
	GRPC     string
}

type Metrics struct {
	m                          sync.Mutex
	Duration                   time.Duration
	TotalTransactionsSentCount uint64
	FailedTransactionsCount    uint64
}

type Account struct {
	Address    types.Address
	PrivateKey *ecdsa.PrivateKey
}

func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(web3.Address(address), -1)
	if err != nil {
		return 0, fmt.Errorf("failed to query initial sender nonce: %v", err)
	}
	return nonce, nil
}

func executeTxn(client *jsonrpc.Client, sender Account, receiver types.Address, value *big.Int, nonce *uint64) error {
	signer := crypto.NewEIP155Signer(100)

	txn, err := signer.SignTx(&types.Transaction{
		From:     sender.Address,
		To:       &receiver,
		Gas:      1000000,
		Value:    value,
		GasPrice: big.NewInt(0x100000),
		Nonce:    *nonce,
		V:        []byte{1}, // it is necessary to encode in rlp
	}, sender.PrivateKey)

	if err != nil {
		return fmt.Errorf("failed to sign transaction: %v", err)
	}

	_, err = client.Eth().SendRawTransaction(txn.MarshalRLP())
	if err != nil {
		return fmt.Errorf("failed to send raw transaction: %v", err)
	}
	atomic.AddUint64(nonce, 1)
	return nil
}

func Run(conf *Configuration, metrics *Metrics) error {
	sender, err := extractSenderAccount(conf.Sender)
	if err != nil {
		return fmt.Errorf("failed to extract sender account: %v", err)
	}

	client, err := createJsonRpcClient(conf.JSONRPC)
	if err != nil {
		return fmt.Errorf("an error has occured while creating JSON-RPC client: %v", err)
	}

	err = shutdownClient(client)
	if err != nil {
		return fmt.Errorf("failed to shutdown JSON-RPC client: %v", err)
	}

	nonce, err := getInitialSenderNonce(client, sender.Address)
	if err != nil {
		return fmt.Errorf("an error occured while getting initial sender nonce: %v", err)
	}

	ticker := time.NewTicker(1 * time.Second / time.Duration(conf.TPS))
	defer ticker.Stop()

	var wg sync.WaitGroup
	defer wg.Wait()

	for i := uint64(0); i < conf.Count; i++ {
		select {
		case <-ticker.C:
			metrics.TotalTransactionsSentCount += 1

			wg.Add(1)
			go func() {
				defer wg.Done()
				err := executeTxn(client, *sender, conf.Receiver, conf.Value, &nonce)
				if err != nil {
					metrics.m.Lock()
					metrics.FailedTransactionsCount += 1
					metrics.m.Unlock()
				}
			}()
		}
	}

	err = waitForTxPool(conf.GRPC)
	if err != nil {
		return fmt.Errorf("an error has occured while waiting for TxPool: %v", err)
	}
	return nil
}
