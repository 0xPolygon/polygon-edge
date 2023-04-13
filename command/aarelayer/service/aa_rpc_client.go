package service

import (
	"context"
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

// AARPCClient provides an interface to access some JSON-RPC methods for Ethereum-compatible nodes
type AARPCClient interface {
	// GetNonce retrieves current nonce for address
	GetNonce(address ethgo.Address) (uint64, error)
	// GetNonce retrieves current aa invoker nonce for address
	GetAANonce(invokerAddress, address ethgo.Address) (uint64, error)
	// GetGasPrice retrieves current gas price for the chain
	GetGasPrice() (uint64, error)
	// SendTransaction sends transaction but does not wait for receipt
	SendTransaction(txSerialized []byte) (ethgo.Hash, error)
	// WaitForReceipt waits for receipt of specific transaction
	WaitForReceipt(ctx context.Context, hash ethgo.Hash, delay time.Duration, numRetries int) (*ethgo.Receipt, error)
}

var _ AARPCClient = (*aaRPCClientImpl)(nil)

type aaRPCClientImpl struct {
	client *jsonrpc.Client
}

func NewAARPCClient(ipAddress string) (AARPCClient, error) {
	client, err := jsonrpc.NewClient(ipAddress)
	if err != nil {
		return nil, err
	}

	return &aaRPCClientImpl{client: client}, nil
}

func (t *aaRPCClientImpl) GetGasPrice() (uint64, error) {
	return t.client.Eth().GasPrice()
}

func (t *aaRPCClientImpl) SendTransaction(txSerialized []byte) (ethgo.Hash, error) {
	return t.client.Eth().SendRawTransaction(txSerialized)
}

func (t *aaRPCClientImpl) WaitForReceipt(
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

func (t *aaRPCClientImpl) GetNonce(address ethgo.Address) (uint64, error) {
	return t.client.Eth().GetNonce(address, ethgo.Pending)
}

func (t *aaRPCClientImpl) GetAANonce(invokerAddress, address ethgo.Address) (uint64, error) {
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
