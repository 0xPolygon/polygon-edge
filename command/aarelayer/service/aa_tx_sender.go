package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const (
	defaultGasPrice   = 1879048192 // 0x70000000
	defaultGasLimit   = 5242880    // 0x500000
	DefaultRPCAddress = "http://127.0.0.1:8545"
)

var (
	errNoAccounts = errors.New("no accounts registered")
)

type AATxSender interface {
	// GetNonce retrieves current nonce for address
	GetNonce(address ethgo.Address) (uint64, error)
	// SendTransaction sends transaction but does not wait for receipt
	SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error)
	// WaitForReceipt waits for receipt of specific transaction
	WaitForReceipt(ctx context.Context, hash ethgo.Hash, delay time.Duration, numRetries int) (*ethgo.Receipt, error)
}

var _ AATxSender = (*AATxSenderImpl)(nil)

type AATxSenderImpl struct {
	client *jsonrpc.Client

	lock sync.Mutex
}

func NewAATxSender(ipAddress string) (AATxSender, error) {
	client, err := jsonrpc.NewClient(ipAddress)
	if err != nil {
		return nil, err
	}

	return &AATxSenderImpl{
		client: client,
	}, nil
}

// SendTransaction sends transaction but does not wait for receipt
func (t *AATxSenderImpl) SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	nonce, err := t.GetNonce(key.Address())
	if err != nil {
		return ethgo.ZeroHash, err
	}

	txn.Nonce = nonce

	if txn.GasPrice == 0 {
		txn.GasPrice, err = t.client.Eth().GasPrice()
		if err != nil {
			return ethgo.ZeroHash, err
		}
	}

	if txn.Gas == 0 {
		txn.Gas = defaultGasLimit
	}

	chainID, err := t.client.Eth().ChainID()
	if err != nil {
		return ethgo.ZeroHash, err
	}

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	if txn, err = signer.SignTx(txn, key); err != nil {
		return ethgo.ZeroHash, err
	}

	data, err := txn.MarshalRLPTo(nil)
	if err != nil {
		return ethgo.ZeroHash, err
	}

	return t.client.Eth().SendRawTransaction(data)
}

func (t *AATxSenderImpl) WaitForReceipt(
	ctx context.Context, hash ethgo.Hash, delay time.Duration, numRetries int) (*ethgo.Receipt, error) {
	ticker := time.NewTicker(delay)
	defer ticker.Stop()

	for i := 0; i < numRetries; i++ {
		receipt, err := t.client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}

		if receipt != nil {
			return receipt, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}

	return nil, fmt.Errorf("timeout while waiting for transaction %s to be processed", hash)
}

func (t *AATxSenderImpl) GetNonce(address ethgo.Address) (uint64, error) {
	return t.client.Eth().GetNonce(address, ethgo.Pending)
}
