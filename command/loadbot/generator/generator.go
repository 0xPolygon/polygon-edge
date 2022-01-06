package generator

import (
	"crypto/ecdsa"
	"github.com/0xPolygon/polygon-sdk/types"
	"math/big"
)

type TransactionGenerator interface {
	GenerateTransaction() (*types.Transaction, error)
	GetExampleTransaction() (*types.Transaction, error)
	GetTransactionErrors() []*FailedTxnInfo
	MarkFailedTxn(failedTxn *FailedTxnInfo)
	SetGasEstimate(gasEstimate uint64)
}

type TxnErrorType string

const (
	ReceiptErrorType TxnErrorType = "ReceiptErrorType"
	AddErrorType     TxnErrorType = "AddErrorType"
)

type TxnError struct {
	Error     error
	ErrorType TxnErrorType
}

type FailedTxnInfo struct {
	Index  uint64
	TxHash string
	Error  *TxnError
}

type GeneratorParams struct {
	Nonce         uint64
	ChainID       uint64
	SenderAddress types.Address
	SenderKey     *ecdsa.PrivateKey
	Value         *big.Int
	GasPrice      *big.Int
}
