package helper

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"time"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"
)

// use a deterministic wallet/private key so that the address of the deployed contracts
// are deterministic
var defKey *wallet.Key

func init() {
	dec, err := hex.DecodeString("aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d")
	if err != nil {
		panic(err)
	}
	defKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		panic(err)
	}
}

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

func GetDefAccount() ethgo.Address {
	return defKey.Address()
}

// SendTxn function sends transaction to the rootchain
// blocks until receipt hash is returned
func SendTxn(client *jsonrpc.Client, nonce uint64, txn *ethgo.Transaction) (*ethgo.Receipt, error) {
	txn.GasPrice = defaultGasPrice
	txn.Gas = defaultGasLimit
	txn.Nonce = nonce

	chainID, err := client.Eth().ChainID()
	if err != nil {
		return nil, err
	}
	signer := wallet.NewEIP155Signer(chainID.Uint64())
	txn, err = signer.SignTx(txn, defKey)
	if err != nil {
		return nil, err
	}
	data, err := txn.MarshalRLPTo(nil)
	if err != nil {
		return nil, err
	}
	txnHash, err := client.Eth().SendRawTransaction(data)
	if err != nil {
		return nil, err
	}

	receipt, err := waitForReceipt(client.Eth(), txnHash)
	if err != nil {
		return nil, err
	}
	return receipt, nil
}

func ExistsCode(addr ethgo.Address) bool {
	ipAddr := ReadRootchainIP()

	provider, err := jsonrpc.NewClient(ipAddr)
	if err != nil {
		panic(err)
	}

	code, err := provider.Eth().GetCode(addr, ethgo.Latest)
	if err != nil {
		panic(err)
	}
	if code == "0x" {
		return false
	}
	return true
}

func FundAccount(account ethgo.Address) error {
	ipAddr := ReadRootchainIP()

	provider, err := jsonrpc.NewClient(ipAddr)
	if err != nil {
		return err
	}

	accounts, err := provider.Eth().Accounts()
	if err != nil {
		return err
	}

	txn := &ethgo.Transaction{
		From:     accounts[0],
		To:       &account,
		GasPrice: defaultGasPrice,
		Gas:      defaultGasLimit,
		Value:    big.NewInt(1000000000000000000),
	}

	txnHash, err := provider.Eth().SendTransaction(txn)
	if err != nil {
		return err
	}
	receipt, err := waitForReceipt(provider.Eth(), txnHash)
	if err != nil {
		return err
	}
	fmt.Printf("Txn Receipt: %v\n", receipt)
	return nil
}

func waitForReceipt(client *jsonrpc.Eth, hash ethgo.Hash) (*ethgo.Receipt, error) {
	var count uint64
	for {
		receipt, err := client.GetTransactionReceipt(hash)
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
