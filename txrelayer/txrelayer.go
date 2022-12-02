package txrelayer

import (
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

var (
	errNoAccounts               = errors.New("no accounts registered")
	errNonSignedTxnNotSupported = errors.New("sending non signed transactions supported only for local account mode")
)

type TxRelayer interface {
	// Call executes a message call immediately without creating a transaction on the blockchain
	Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error)
	// SendTransaction signs given transaction by provided key and sends it to the blockchain
	SendTransaction(transaction *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error)
	// GetNonce queries nonce for the provided account
	GetNonce(address ethgo.Address) (uint64, error)
}

type TxRelayerImpl struct {
	client       *jsonrpc.Client
	localAccount bool
}

func NewTxRelayer(ipAddr string, opts ...TxRelayerOption) (*TxRelayerImpl, error) {
	client, err := jsonrpc.NewClient(ipAddr)
	if err != nil {
		return nil, err
	}

	t := &TxRelayerImpl{client: client}
	for _, opt := range opts {
		opt(t)
	}

	return t, nil
}

func NewTxRelayerWithClient(client *jsonrpc.Client, opts ...TxRelayerOption) *TxRelayerImpl {
	t := &TxRelayerImpl{client: client}
	for _, opt := range opts {
		opt(t)
	}

	return t
}

// Call executes a message call immediately without creating a transaction on the blockchain
func (t *TxRelayerImpl) Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error) {
	callMsg := &ethgo.CallMsg{
		From:     from,
		To:       &to,
		Data:     input,
		GasPrice: defaultGasPrice,
		Gas:      big.NewInt(defaultGasLimit),
	}

	return t.client.Eth().Call(callMsg, ethgo.Pending)
}

// SendTransaction signs given transaction by provided key and sends it to the blockchain
func (t *TxRelayerImpl) SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	var (
		txnHash ethgo.Hash
		err     error
	)

	if !t.localAccount {
		txnHash, err = t.sendSignedTransaction(txn, key)
		if err != nil {
			return nil, err
		}
	} else {
		txnHash, err = t.sendNonSignedTransaction(txn)
		if err != nil {
			return nil, err
		}
	}

	return t.waitForReceipt(txnHash)
}

// GetNonce queries nonce for the provided account
func (t *TxRelayerImpl) GetNonce(address ethgo.Address) (uint64, error) {
	return t.client.Eth().GetNonce(address, ethgo.Pending)
}

// sendSignedTransaction sends signed transaction
func (t *TxRelayerImpl) sendSignedTransaction(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	if txn.Nonce == 0 {
		pendingNonce, err := t.client.Eth().GetNonce(txn.From, ethgo.Pending)
		if err != nil {
			return ethgo.ZeroHash, err
		}

		txn.Nonce = pendingNonce
	}

	if txn.GasPrice == 0 {
		txn.GasPrice = defaultGasPrice
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

// sendNonSignedTransaction sends non signed transaction, where sender is the first of available accounts.
// It can be invoked only with enabled local account mode
func (t *TxRelayerImpl) sendNonSignedTransaction(txn *ethgo.Transaction) (ethgo.Hash, error) {
	if !t.localAccount {
		return ethgo.ZeroHash, errNonSignedTxnNotSupported
	}

	accounts, err := t.client.Eth().Accounts()
	if err != nil {
		return ethgo.ZeroHash, err
	}

	if len(accounts) == 0 {
		return ethgo.ZeroHash, errNoAccounts
	}

	txn.From = accounts[0]

	return t.client.Eth().SendTransaction(txn)
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

		if count > 100 {
			return nil, fmt.Errorf("timeout while waiting for transaction %s to be processed", hash)
		}

		time.Sleep(50 * time.Millisecond)
		count++
	}
}

type TxRelayerOption func(*TxRelayerImpl)

func WithLocalAccount() TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.localAccount = true
	}
}
