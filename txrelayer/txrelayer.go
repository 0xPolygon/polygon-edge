package txrelayer

import (
	"fmt"
	"math/big"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

type RelayerOption func(*TxRelayer)

func WithLocalAccount() RelayerOption {
	return func(h *TxRelayer) {
		h.localAcct = true
	}
}

type TxRelayer struct {
	client *jsonrpc.Client

	// localAcct set to true means that the relayer will use a
	// local account from eth_accounts as the sender address
	// This only works when running geth in development mode.
	localAcct bool
}

func NewTxRelayer(addr string, opts ...RelayerOption) (*TxRelayer, error) {
	client, err := jsonrpc.NewClient(addr)
	if err != nil {
		return nil, err
	}

	t := &TxRelayer{
		client: client,
	}
	for _, opt := range opts {
		opt(t)
	}
	return t, nil
}

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

// SendTxnLocal the relayer will use a local account from eth_accounts
// as the sender address. This only works when running geth in development mode.
func (t *TxRelayer) SendTxnLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	// TODO: Remove
	accounts, err := t.client.Eth().Accounts()
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, fmt.Errorf("no accounts registered")
	}
	txn.From = accounts[0]

	txnHash, err := t.client.Eth().SendTransaction(txn)
	if err != nil {
		return nil, err
	}

	receipt, err := t.waitForReceipt(txnHash)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (t *TxRelayer) SendTxn(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error) {
	pendingNonce, err := t.client.Eth().GetNonce(key.Address(), ethgo.Pending)
	if err != nil {
		return nil, err
	}

	txn.GasPrice = defaultGasPrice
	txn.Gas = defaultGasLimit
	txn.Nonce = pendingNonce

	chainID, err := t.client.Eth().ChainID()
	if err != nil {
		return nil, err
	}

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	if txn, err = signer.SignTx(txn, key); err != nil {
		return nil, err
	}

	data, err := txn.MarshalRLPTo(nil)
	if err != nil {
		return nil, err
	}

	txnHash, err := t.client.Eth().SendRawTransaction(data)
	if err != nil {
		return nil, err
	}

	receipt, err := t.waitForReceipt(txnHash)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func (t *TxRelayer) waitForReceipt(hash ethgo.Hash) (*ethgo.Receipt, error) {
	var count uint64

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
			return nil, fmt.Errorf("timeout")
		}

		time.Sleep(50 * time.Millisecond)
		count++
	}
}

// Call function is used to query a smart contract on given 'to' address
func (t *TxRelayer) Call(from, to ethgo.Address, input []byte) (string, error) {
	callMsg := &ethgo.CallMsg{
		From:     from,
		To:       &to,
		Data:     input,
		GasPrice: defaultGasPrice,
		Gas:      big.NewInt(defaultGasLimit),
	}

	return t.client.Eth().Call(callMsg, ethgo.Pending)
}
