package helper

import (
	"encoding/hex"
	"math/big"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
	"github.com/umbracle/ethgo/wallet"

	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

var (
	// TODO: @Stefan-Ethernal Use either private key provided through CLI input or this (denoting dev vs prod mode)
	// use a deterministic wallet/private key so that the address of the deployed contracts
	// are deterministic
	rootchainAdminKey *wallet.Key
)

func init() {
	dec, err := hex.DecodeString("aa75e9a7d427efc732f8e4f1a5b7646adcc61fd5bae40f80d13c8419c9f43d6d")
	if err != nil {
		panic(err)
	}

	rootchainAdminKey, err = wallet.NewWalletFromPrivKey(dec)
	if err != nil {
		panic(err)
	}
}

func GetRootchainAdminAddr() types.Address {
	return types.Address(rootchainAdminKey.Address())
}

func GetRootchainAdminKey() ethgo.Key {
	return rootchainAdminKey
}

func ContractExists(client *jsonrpc.Client, contractAddr types.Address) (bool, error) {
	code, err := client.Eth().GetCode(ethgo.Address(contractAddr), ethgo.Latest)
	if err != nil {
		return false, err
	}

	return code != "0x", nil
}

// FundAccount funds provided account.
// Used only for testing purposes.
func FundAccount(ipAddress string, address types.Address) (types.Hash, error) {
	txRelayer, err := txrelayer.NewTxRelayer(ipAddress, txrelayer.WithLocalAccount())
	if err != nil {
		return types.ZeroHash, err
	}

	fundAddr := ethgo.Address(address)
	txn := &ethgo.Transaction{
		To:    &fundAddr,
		Value: big.NewInt(1000000000000000000),
	}

	receipt, err := txRelayer.SendTransaction(txn, GetRootchainAdminKey())
	if err != nil {
		return types.ZeroHash, err
	}

	return types.Hash(receipt.TransactionHash), nil
}
