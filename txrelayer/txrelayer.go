package txrelayer

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const (
	DefaultGasPrice   = 1879048192 // 0x70000000
	DefaultGasLimit   = 5242880    // 0x500000
	DefaultRPCAddress = "http://127.0.0.1:8545"
	numRetries        = 1000
)

var (
	errNoAccounts = errors.New("no accounts registered")
)

type TxRelayer interface {
	// Call executes a message call immediately without creating a transaction on the blockchain
	Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error)
	// SendTransaction signs given transaction by provided key and sends it to the blockchain
	SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error)
	// SendTransactionLocal sends non-signed transaction
	// (this function is meant only for testing purposes and is about to be removed at some point)
	SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error)
}

var _ TxRelayer = (*TxRelayerImpl)(nil)

type TxRelayerImpl struct {
	ipAddress      string
	client         *jsonrpc.Client
	receiptTimeout time.Duration

	lock sync.Mutex
}

func NewTxRelayer(opts ...TxRelayerOption) (TxRelayer, error) {
	t := &TxRelayerImpl{
		ipAddress:      "http://127.0.0.1:8545",
		receiptTimeout: 50 * time.Millisecond,
	}
	for _, opt := range opts {
		opt(t)
	}

	if t.client == nil {
		client, err := jsonrpc.NewClient(t.ipAddress)
		if err != nil {
			return nil, err
		}

		t.client = client
	}

	return t, nil
}

// Call executes a message call immediately without creating a transaction on the blockchain
func (t *TxRelayerImpl) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	callMsg := &ethgo.CallMsg{
		From: from,
		To:   &to,
		Data: input,
	}

	return t.client.Eth().Call(callMsg, ethgo.Pending)
}

// SendTransaction signs given transaction by provided key and sends it to the blockchain
func (t *TxRelayerImpl) SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	txnHash, err := t.sendTransactionLocked(txn, key)
	if err != nil {
		return nil, err
	}

	return t.waitForReceipt(txnHash)
}

func (t *TxRelayerImpl) sendTransactionLocked(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	nonce, err := t.client.Eth().GetNonce(key.Address(), ethgo.Pending)
	if err != nil {
		return ethgo.ZeroHash, err
	}

	txn.Nonce = nonce

	if txn.GasPrice == 0 {
		txn.GasPrice = DefaultGasPrice
	}

	if txn.Gas == 0 {
		txn.Gas = DefaultGasLimit
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

// SendTransactionLocal sends non-signed transaction
// (this function is meant only for testing purposes and is about to be removed at some point)
func (t *TxRelayerImpl) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	accounts, err := t.client.Eth().Accounts()
	if err != nil {
		return nil, err
	}

	if len(accounts) == 0 {
		return nil, errNoAccounts
	}

	txn.From = accounts[0]
	txn.Gas = DefaultGasLimit
	txn.GasPrice = DefaultGasPrice

	txnHash, err := t.client.Eth().SendTransaction(txn)
	if err != nil {
		return nil, err
	}

	return t.waitForReceipt(txnHash)
}

func (t *TxRelayerImpl) waitForReceipt(hash ethgo.Hash) (*ethgo.Receipt, error) {
	count := uint(0)

	for {
		receipt, err := t.client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}

		if receipt != nil {
			return receipt, nil
		}

		if count > numRetries {
			return nil, fmt.Errorf("timeout while waiting for transaction %s to be processed", hash)
		}

		time.Sleep(t.receiptTimeout)
		count++
	}
}

type TxRelayerOption func(*TxRelayerImpl)

func WithClient(client *jsonrpc.Client) TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.client = client
	}
}

func WithIPAddress(ipAddress string) TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.ipAddress = ipAddress
	}
}

func WithReceiptTimeout(receiptTimeout time.Duration) TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.receiptTimeout = receiptTimeout
	}
}
