package loadbot

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

type Account struct {
	Address    types.Address
	PrivateKey *ecdsa.PrivateKey
}
type Configuration struct {
	TPS      uint64
	Sender   types.Address
	Receiver types.Address
	Value    *big.Int
	Count    uint64
	JSONRPC  string
}

type Metrics struct {
	TotalTransactionsSentCount uint64
	FailedTransactionsCount    uint64
}

type Loadbot struct {
	cfg     *Configuration
	metrics *Metrics
}

func NewLoadBot(cfg *Configuration, metrics *Metrics) *Loadbot {
	return &Loadbot{cfg: cfg, metrics: metrics}
}
func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(web3.Address(address), web3.Latest)
	if err != nil {
		return 0, fmt.Errorf("failed to query initial sender nonce: %v", err)
	}
	return nonce, nil
}

func executeTxn(client *jsonrpc.Client, sender Account, receiver types.Address, value *big.Int, nonce uint64) (web3.Hash, error) {
	signer := crypto.NewEIP155Signer(100)

	txn, err := signer.SignTx(&types.Transaction{
		From:     sender.Address,
		To:       &receiver,
		Gas:      1000000,
		Value:    value,
		GasPrice: big.NewInt(0x100000),
		Nonce:    nonce,
		V:        big.NewInt(1), // it is necessary to encode in rlp
	}, sender.PrivateKey)

	if err != nil {
		return web3.Hash{}, fmt.Errorf("failed to sign transaction: %v", err)
	}

	hash, err := client.Eth().SendRawTransaction(txn.MarshalRLP())
	if err != nil {
		return web3.Hash{}, fmt.Errorf("failed to send raw transaction: %v", err)
	}
	return hash, nil
}

func (l *Loadbot) Run() error {
	sender, err := extractSenderAccount(l.cfg.Sender)
	if err != nil {
		return fmt.Errorf("failed to extract sender account: %v", err)
	}

	client, err := createJsonRpcClient(l.cfg.JSONRPC)
	if err != nil {
		return fmt.Errorf("an error has occured while creating JSON-RPC client: %v", err)
	}
	defer shutdownClient(client)

	nonce, err := getInitialSenderNonce(client, sender.Address)
	if err != nil {
		return fmt.Errorf("an error occured while getting initial sender nonce: %v", err)
	}

	ticker := time.NewTicker(1 * time.Second / time.Duration(l.cfg.TPS))
	defer ticker.Stop()

	var wg sync.WaitGroup

	for i := uint64(0); i < l.cfg.Count; i++ {
		<-ticker.C

		l.metrics.TotalTransactionsSentCount += 1

		wg.Add(1)
		go func() {
			defer wg.Done()

			// take nonce first
			newNextNonce := atomic.AddUint64(&nonce, 1)
			txHash, err := executeTxn(client, *sender, l.cfg.Receiver, l.cfg.Value, newNextNonce-1)
			if err != nil {
				atomic.AddUint64(&l.metrics.FailedTransactionsCount, 1)
				return
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			_, err = tests.WaitForReceipt(ctx, client.Eth(), txHash)
			if err != nil {
				atomic.AddUint64(&l.metrics.FailedTransactionsCount, 1)
				return
			}
		}()
	}

	wg.Wait()
	return nil
}
