package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const defaultGasLimit = 5242880 // 0x500000

type AATxSender interface {
	// GetNonce retrieves current nonce for address
	GetNonce(address ethgo.Address) (uint64, error)
	// GetNonce retrieves current aa invoker nonce for address
	GetAANonce(invokerAddress, address ethgo.Address) (uint64, error)
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

	if txn.GasPrice == 0 {
		gasPrice, err := t.client.Eth().GasPrice()
		if err != nil {
			return ethgo.ZeroHash, err
		}

		txn.GasPrice = gasPrice
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

func (t *AATxSenderImpl) GetAANonce(invokerAddress, address ethgo.Address) (uint64, error) {
	data, err := aaInvokerNoncesAbiType.Encode([]interface{}{address})
	if err != nil {
		return 0, nil
	}

	callMsg := &ethgo.CallMsg{
		To:   &invokerAddress,
		Data: data,
	}

	response, err := t.client.Eth().Call(callMsg, ethgo.Pending)
	if err != nil {
		return 0, err
	}

	nonce, err := types.ParseUint256orHex(&response)
	if err != nil {
		return 0, fmt.Errorf("unable to decode hex response, %w", err)
	}

	return nonce.Uint64(), nil
}
