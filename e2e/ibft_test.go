package e2e

import (
	"context"
	"github.com/0xPolygon/minimal/consensus/ibft"
	"github.com/umbracle/go-web3"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestIbft_Transfer(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	ibftManager := framework.NewIBFTServersManager(t, IBFTMinNodes, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.Premine(senderAddr, framework.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	for i := 0; i < IBFTMinNodes-1; i++ {
		txn := &types.Transaction{
			From:     senderAddr,
			To:       &receiverAddr,
			GasPrice: big.NewInt(10000),
			Gas:      1000000,
			Value:    framework.EthToWei(1),
			Nonce:    uint64(i),
		}
		txn, err := signer.SignTx(txn, senderKey)
		if err != nil {
			t.Fatal(err)
		}
		data := txn.MarshalRLP()

		hash, err := srv.JSONRPC().Eth().SendRawTransaction(data)
		assert.NoError(t, err)
		assert.NotNil(t, hash)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		receipt, err := srv.WaitForReceipt(ctx, hash)

		assert.NoError(t, err)
		assert.NotNil(t, receipt)
		assert.Equal(t, receipt.TransactionHash, hash)
	}
}

func TestIbft_TransactionFeeRecipient(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	senderKey, senderAddr := framework.GenerateKeyAndAddr(t)
	_, receiverAddr := framework.GenerateKeyAndAddr(t)

	ibftManager := framework.NewIBFTServersManager(t, IBFTMinNodes, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.Premine(senderAddr, framework.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)
	clt := srv.JSONRPC()

	// get latest nonce
	lastNonce, err := clt.Eth().GetNonce(web3.Address(senderAddr), web3.Latest)
	assert.NoError(t, err)

	txn := &types.Transaction{
		From:     senderAddr,
		To:       &receiverAddr,
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    framework.EthToWei(1),
		Nonce:    lastNonce,
	}
	txn, err = signer.SignTx(txn, senderKey)
	if err != nil {
		t.Fatal(err)
	}
	data := txn.MarshalRLP()

	hash, err := clt.Eth().SendRawTransaction(data)
	assert.NoError(t, err)
	assert.NotNil(t, hash)

	ctx1, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := srv.WaitForReceipt(ctx1, hash)

	assert.NoError(t, err)
	assert.NotNil(t, receipt)
	assert.Equal(t, receipt.TransactionHash, hash)

	// Get the block proposer from the extra data seal
	assert.NotNil(t, receipt.BlockHash)
	block, err := clt.Eth().GetBlockByHash(receipt.BlockHash, false)
	assert.NoError(t, err)
	extraData := &ibft.IstanbulExtra{}
	extraDataWithoutVanity := block.ExtraData[ibft.IstanbulExtraVanity:]
	err = extraData.UnmarshalRLP(extraDataWithoutVanity)
	assert.NoError(t, err)

	proposerAddr, err := framework.EcrecoverFromBlockhash(types.Hash(block.Hash), extraData.Seal)
	assert.NoError(t, err)

	// Given that this is the first transaction on the blockchain, proposer's balance should be equal to the tx fee
	balanceProposer, err := clt.Eth().GetBalance(web3.Address(proposerAddr), web3.Latest)
	assert.NoError(t, err)

	txFee := new(big.Int).Mul(new(big.Int).SetUint64(receipt.GasUsed), txn.GasPrice)
	assert.Equalf(t, txFee, balanceProposer, "Proposer didn't get appropriate transaction fee")
}
