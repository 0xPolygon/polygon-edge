package txrelayer

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

const (
	defaultGasPrice            = 1879048192 // 0x70000000
	DefaultGasLimit            = 5242880    // 0x500000
	DefaultRPCAddress          = "http://127.0.0.1:8545"
	defaultNumRetries          = 1000
	gasLimitIncreasePercentage = 100
	feeIncreasePercentage      = 100
)

var (
	errNoAccounts     = errors.New("no accounts registered")
	errMethodNotFound = errors.New("method not found")

	// dynamicFeeTxFallbackErrs represents known errors which are the reason to fallback
	// from sending dynamic fee tx to legacy tx
	dynamicFeeTxFallbackErrs = []error{types.ErrTxTypeNotSupported, errMethodNotFound}
)

type TxRelayer interface {
	// Call executes a message call immediately without creating a transaction on the blockchain
	Call(from ethgo.Address, to ethgo.Address, input []byte) (string, error)
	// SendTransaction signs given transaction by provided key and sends it to the blockchain
	SendTransaction(txn *ethgo.Transaction, key ethgo.Key) (*ethgo.Receipt, error)
	// SendTransactionLocal sends non-signed transaction
	// (this function is meant only for testing purposes and is about to be removed at some point)
	SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error)
	// Client returns jsonrpc client
	Client() *jsonrpc.Client
}

var _ TxRelayer = (*TxRelayerImpl)(nil)

type TxRelayerImpl struct {
	ipAddress      string
	client         *jsonrpc.Client
	receiptTimeout time.Duration
	numRetries     int

	lock sync.Mutex

	writer io.Writer
}

func NewTxRelayer(opts ...TxRelayerOption) (TxRelayer, error) {
	t := &TxRelayerImpl{
		ipAddress:      DefaultRPCAddress,
		receiptTimeout: 50 * time.Millisecond,
		numRetries:     defaultNumRetries,
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
		if txn.Type != ethgo.TransactionLegacy {
			for _, fallbackErr := range dynamicFeeTxFallbackErrs {
				if strings.Contains(
					strings.ToLower(err.Error()),
					strings.ToLower(fallbackErr.Error())) {
					// "downgrade" transaction to the legacy tx type and resend it
					txn.Type = ethgo.TransactionLegacy
					txn.GasPrice = 0

					return t.SendTransaction(txn, key)
				}
			}
		}

		return nil, err
	}

	return t.waitForReceipt(txnHash)
}

// Client returns jsonrpc client
func (t *TxRelayerImpl) Client() *jsonrpc.Client {
	return t.client
}

func (t *TxRelayerImpl) sendTransactionLocked(txn *ethgo.Transaction, key ethgo.Key) (ethgo.Hash, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	nonce, err := t.client.Eth().GetNonce(key.Address(), ethgo.Pending)
	if err != nil {
		return ethgo.ZeroHash, fmt.Errorf("failed to get nonce: %w", err)
	}

	chainID, err := t.client.Eth().ChainID()
	if err != nil {
		return ethgo.ZeroHash, err
	}

	txn.ChainID = chainID
	txn.Nonce = nonce

	if txn.From == ethgo.ZeroAddress {
		txn.From = key.Address()
	}

	if txn.Type == ethgo.TransactionDynamicFee {
		maxPriorityFee := txn.MaxPriorityFeePerGas
		if maxPriorityFee == nil {
			// retrieve the max priority fee per gas
			if maxPriorityFee, err = t.Client().Eth().MaxPriorityFeePerGas(); err != nil {
				return ethgo.ZeroHash, fmt.Errorf("failed to get max priority fee per gas: %w", err)
			}

			// set retrieved max priority fee per gas increased by certain percentage
			compMaxPriorityFee := new(big.Int).Mul(maxPriorityFee, big.NewInt(feeIncreasePercentage))
			compMaxPriorityFee = compMaxPriorityFee.Div(compMaxPriorityFee, big.NewInt(100))
			txn.MaxPriorityFeePerGas = new(big.Int).Add(maxPriorityFee, compMaxPriorityFee)
		}

		if txn.MaxFeePerGas == nil {
			// retrieve the latest base fee
			feeHist, err := t.Client().Eth().FeeHistory(1, ethgo.Latest, nil)
			if err != nil {
				return ethgo.ZeroHash, fmt.Errorf("failed to get fee history: %w", err)
			}

			baseFee := feeHist.BaseFee[len(feeHist.BaseFee)-1]
			// set max fee per gas as sum of base fee and max priority fee
			// (increased by certain percentage)
			maxFeePerGas := new(big.Int).Add(baseFee, maxPriorityFee)
			compMaxFeePerGas := new(big.Int).Mul(maxFeePerGas, big.NewInt(feeIncreasePercentage))
			compMaxFeePerGas = compMaxFeePerGas.Div(compMaxFeePerGas, big.NewInt(100))
			txn.MaxFeePerGas = new(big.Int).Add(maxFeePerGas, compMaxFeePerGas)
		}
	} else if txn.GasPrice == 0 {
		gasPrice, err := t.Client().Eth().GasPrice()
		if err != nil {
			return ethgo.ZeroHash, fmt.Errorf("failed to get gas price: %w", err)
		}

		txn.GasPrice = gasPrice + (gasPrice * feeIncreasePercentage / 100)
	}

	if txn.Gas == 0 {
		gasLimit, err := t.client.Eth().EstimateGas(ConvertTxnToCallMsg(txn))
		if err != nil {
			return ethgo.ZeroHash, fmt.Errorf("failed to estimate gas: %w", err)
		}

		txn.Gas = gasLimit + (gasLimit * gasLimitIncreasePercentage / 100)
	}

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	if txn, err = signer.SignTx(txn, key); err != nil {
		return ethgo.ZeroHash, err
	}

	data, err := txn.MarshalRLPTo(nil)
	if err != nil {
		return ethgo.ZeroHash, err
	}

	if t.writer != nil {
		var msg string

		if txn.Type == ethgo.TransactionDynamicFee {
			msg = fmt.Sprintf("[TxRelayer.SendTransaction]\nFrom = %s\nGas = %d\n"+
				"Max Fee Per Gas = %d\nMax Priority Fee Per Gas = %d\n",
				txn.From, txn.Gas, txn.MaxFeePerGas, txn.MaxPriorityFeePerGas)
		} else {
			msg = fmt.Sprintf("[TxRelayer.SendTransaction]\nFrom = %s\nGas = %d\nGas Price = %d\n",
				txn.From, txn.Gas, txn.GasPrice)
		}

		_, _ = t.writer.Write([]byte(msg))
	}

	return t.client.Eth().SendRawTransaction(data)
}

// SendTransactionLocal sends non-signed transaction
// (this function is meant only for testing purposes and is about to be removed at some point)
func (t *TxRelayerImpl) SendTransactionLocal(txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	txnHash, err := t.sendTransactionLocalLocked(txn)
	if err != nil {
		return nil, err
	}

	return t.waitForReceipt(txnHash)
}

func (t *TxRelayerImpl) sendTransactionLocalLocked(txn *ethgo.Transaction) (ethgo.Hash, error) {
	t.lock.Lock()
	defer t.lock.Unlock()

	accounts, err := t.client.Eth().Accounts()
	if err != nil {
		return ethgo.ZeroHash, err
	}

	if len(accounts) == 0 {
		return ethgo.ZeroHash, errNoAccounts
	}

	txn.From = accounts[0]

	gasLimit, err := t.client.Eth().EstimateGas(ConvertTxnToCallMsg(txn))
	if err != nil {
		return ethgo.ZeroHash, err
	}

	txn.Gas = gasLimit
	txn.GasPrice = defaultGasPrice

	return t.client.Eth().SendTransaction(txn)
}

func (t *TxRelayerImpl) waitForReceipt(hash ethgo.Hash) (*ethgo.Receipt, error) {
	// A negative numRetries means we don't want to receive the receipt after SendTransaction/SendTransactionLocal calls
	if t.numRetries < 0 {
		return nil, nil
	}

	for count := 0; count < t.numRetries; count++ {
		receipt, err := t.client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}

		if receipt != nil {
			return receipt, nil
		}

		time.Sleep(t.receiptTimeout)
	}

	return nil, fmt.Errorf("timeout while waiting for transaction %s to be processed", hash)
}

// ConvertTxnToCallMsg converts txn instance to call message
func ConvertTxnToCallMsg(txn *ethgo.Transaction) *ethgo.CallMsg {
	return &ethgo.CallMsg{
		From:     txn.From,
		To:       txn.To,
		Data:     txn.Input,
		GasPrice: txn.GasPrice,
		Value:    txn.Value,
		Gas:      new(big.Int).SetUint64(txn.Gas),
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

func WithWriter(writer io.Writer) TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.writer = writer
	}
}

// WithNumRetries sets the maximum number of eth_getTransactionReceipt retries
// before considering the transaction sending as timed out. Set to -1 to disable
// waitForReceipt and not wait for the transaction receipt
func WithNumRetries(numRetries int) TxRelayerOption {
	return func(t *TxRelayerImpl) {
		t.numRetries = numRetries
	}
}
