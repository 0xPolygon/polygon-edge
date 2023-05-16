package benchmark

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

// deployContractOnRootAndChild deploys contract code on both root and child chain
func deployContractOnRootAndChild(
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
	contractChildAddr := deployContract(b, childTxRelayer, sender, byteCode)

	// deploy contract on the root chain
	contractRootAddr := deployContract(b, rootTxRelayer, sender, byteCode)

	return contractChildAddr, contractRootAddr
}

// deployContract deploys contract code for the given relayer
func deployContract(b *testing.B, txRelayer txrelayer.TxRelayer, sender ethgo.Key, byteCode []byte) ethgo.Address {
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

// getTxInput returns input for sending tx, given the abi encoded method and call parameters
func getTxInput(b *testing.B, method *abi.Method, args interface{}) []byte {
	b.Helper()

	var (
		input []byte
		err   error
	)

	if args != nil {
		input, err = method.Encode(args)
	} else {
		input = method.ID()
	}

	require.NoError(b, err)

	return input
}

// setContractDependencyAddress calls setContract function on caller contract, to set address of the callee contract
func setContractDependencyAddress(b *testing.B, txRelayer txrelayer.TxRelayer, callerContractAddr ethgo.Address,
	calleeContractAddr ethgo.Address, setContractAbiMethod *abi.Method, sender ethgo.Key) {
	b.Helper()

	input := getTxInput(b, setContractAbiMethod, []interface{}{calleeContractAddr})
	receipt, err := txRelayer.SendTransaction(
		&ethgo.Transaction{
			To:    &callerContractAddr,
			Input: input,
		}, sender)
	require.NoError(b, err)
	require.Equal(b, uint64(types.ReceiptSuccess), receipt.Status)
}

// getPrivateKey initializes a private key from provided raw private key
func getPrivateKey(b *testing.B, privateKeyRaw string) ethgo.Key {
	b.Helper()

	dec, err := hex.DecodeString(privateKeyRaw)
	require.NoError(b, err)

	privateKey, err := wallet.NewWalletFromPrivKey(dec)
	require.NoError(b, err)

	return privateKey
}
