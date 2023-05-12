package common

import (
	"encoding/hex"
	"testing"

	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/wallet"
)

// DeployContractOnRootAndChild deploys contract code on both root and child chain
func DeployContractOnRootAndChild(
	b *testing.B,
	childTxRelayer txrelayer.TxRelayer,
	rootTxRelayer txrelayer.TxRelayer,
	sender ethgo.Key,
	byteCodeString string) (ethgo.Address, ethgo.Address) {
	b.Helper()

	// bytecode from string
	byteCode, err := hex.DecodeString(byteCodeString)
	require.NoError(b, err)

	// deploy contract on the child chain
	contractChildAddr := DeployContract(b, childTxRelayer, sender, byteCode)

	// deploy contract on the root chain
	contractRootAddr := DeployContract(b, rootTxRelayer, sender, byteCode)

	return contractChildAddr, contractRootAddr
}

// DeployContract deploys contract code for the given relayer
func DeployContract(b *testing.B, txRelayer txrelayer.TxRelayer, sender ethgo.Key, byteCode []byte) ethgo.Address {
	b.Helper()

	txn := &ethgo.Transaction{
		To:    nil, // contract deployment
		Input: byteCode,
	}

	receipt, err := txRelayer.SendTransaction(txn, sender)
	require.NoError(b, err)
	require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
	require.NotEqual(b, ethgo.ZeroAddress, receipt.ContractAddress)

	return receipt.ContractAddress
}

// GetTxInput returns input for sending tx, given the abi encoded method and call parameters
func GetTxInput(b *testing.B, method *abi.Method, args interface{}) []byte {
	b.Helper()

	var input []byte

	var err error

	if args != nil {
		input, err = method.Encode(args)
	} else {
		input = method.ID()
	}

	require.NoError(b, err)

	return input
}

// SetContractDependencyAddress calls setContract function on caller contract, to set address of the callee contract
func SetContractDependencyAddress(b *testing.B, txRelayer txrelayer.TxRelayer, callerContractAddr ethgo.Address,
	calleeContractAddr ethgo.Address, setContractAbiMethod *abi.Method, sender ethgo.Key) {
	b.Helper()

	input := GetTxInput(b, setContractAbiMethod, []interface{}{calleeContractAddr})
	receipt, err := txRelayer.SendTransaction(
		&ethgo.Transaction{
			To:    &callerContractAddr,
			Input: input,
		}, sender)
	require.NoError(b, err)
	require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
}

// GetPrivateKey initializes a private key from provided raw private key
func GetPrivateKey(b *testing.B, privateKeyRaw string) ethgo.Key {
	b.Helper()

	dec, err := hex.DecodeString(privateKeyRaw)
	require.NoError(b, err)

	privateKey, err := wallet.NewWalletFromPrivKey(dec)
	require.NoError(b, err)

	return privateKey
}
