package helper

import (
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

type RootchainInteractor interface {
	Call(from types.Address, to types.Address, input []byte) (string, error)
	SendTransaction(transaction *ethgo.Transaction, signer ethgo.Key) (*ethgo.Receipt, error)
	GetPendingNonce(address types.Address) (uint64, error)
	ExistsCode(contractAddr types.Address) (bool, error)
	FundAccount(account types.Address) (types.Hash, error)
}

var _ RootchainInteractor = (*DefaultRootchainInteractor)(nil)

type DefaultRootchainInteractor struct {
	provider *jsonrpc.Client
}

func NewDefaultRootchainInteractor(ipAddress string) (*DefaultRootchainInteractor, error) {
	provider, err := jsonrpc.NewClient(ipAddress)
	if err != nil {
		return nil, err
	}

	return &DefaultRootchainInteractor{provider: provider}, nil
}

func (d *DefaultRootchainInteractor) Call(from types.Address, to types.Address, input []byte) (string, error) {
	toAddr := ethgo.Address(to)
	callMsg := &ethgo.CallMsg{
		From:     ethgo.Address(from),
		To:       &toAddr,
		Data:     input,
		GasPrice: defaultGasPrice,
		Gas:      big.NewInt(defaultGasLimit),
	}

	return d.provider.Eth().Call(callMsg, ethgo.Pending)
}

func (d *DefaultRootchainInteractor) SendTransaction(txn *ethgo.Transaction,
	privKey ethgo.Key) (*ethgo.Receipt, error) {
	if txn.GasPrice == 0 {
		txn.GasPrice = defaultGasPrice
	}

	if txn.Gas == 0 {
		txn.Gas = defaultGasLimit
	}

	chainID, err := d.provider.Eth().ChainID()
	if err != nil {
		return nil, err
	}

	signer := wallet.NewEIP155Signer(chainID.Uint64())
	if txn, err = signer.SignTx(txn, privKey); err != nil {
		return nil, err
	}

	data, err := txn.MarshalRLPTo(nil)
	if err != nil {
		return nil, err
	}

	txnHash, err := d.provider.Eth().SendRawTransaction(data)
	if err != nil {
		return nil, err
	}

	receipt, err := d.waitForReceipt(txnHash)
	if err != nil {
		return nil, err
	}

	return receipt, nil
}

func (d *DefaultRootchainInteractor) GetPendingNonce(address types.Address) (uint64, error) {
	nonce, err := d.provider.Eth().GetNonce(ethgo.Address(address), ethgo.Pending)
	if err != nil {
		return 0, err
	}

	return nonce, nil
}

func (d *DefaultRootchainInteractor) ExistsCode(contractAddr types.Address) (bool, error) {
	code, err := d.provider.Eth().GetCode(ethgo.Address(contractAddr), ethgo.Latest)
	if err != nil {
		return false, err
	}

	return code != "0x", nil
}

func (d *DefaultRootchainInteractor) FundAccount(account types.Address) (types.Hash, error) {
	accounts, err := d.provider.Eth().Accounts()
	if err != nil {
		return types.Hash{}, err
	}

	acc := ethgo.HexToAddress(account.String())
	txn := &ethgo.Transaction{
		From:     accounts[0],
		To:       &acc,
		GasPrice: defaultGasPrice,
		Gas:      defaultGasLimit,
		Value:    big.NewInt(1000000000000000000),
	}

	txnHash, err := d.provider.Eth().SendTransaction(txn)
	if err != nil {
		return types.Hash{}, err
	}

	receipt, err := d.waitForReceipt(txnHash)
	if err != nil {
		return types.Hash{}, err
	}

	return types.BytesToHash(receipt.TransactionHash.Bytes()), nil
}

func (d *DefaultRootchainInteractor) waitForReceipt(hash ethgo.Hash) (*ethgo.Receipt, error) {
	var count uint64

	for {
		receipt, err := d.provider.Eth().GetTransactionReceipt(hash)
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
