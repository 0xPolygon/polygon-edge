package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func findValidatorByAddress(validators []*proto.Snapshot_Validator, addr string) *proto.Snapshot_Validator {
	for _, v := range validators {
		if v.Address == addr {
			return v
		}
	}
	return nil
}

func TestPoS_Stake(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	stakerKey, stakerAddr := framework.GenerateKeyAndAddr(t)
	stakingContractAddr := types.StringToAddress("1001")

	numGenesisValidators := IBFTMinNodes
	ibftManager := framework.NewIBFTServersManager(t, numGenesisValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.Premine(stakerAddr, framework.EthToWei(10))
		config.PremineValidatorBalance(big.NewInt(0), framework.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)

	// Stake Balance
	txn := &types.Transaction{
		From:     stakerAddr,
		To:       &stakingContractAddr,
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    framework.EthToWei(1),
		Nonce:    0,
	}
	txn, err := signer.SignTx(txn, stakerKey)
	assert.NoError(t, err)

	data := txn.MarshalRLP()
	hash, err := srv.JSONRPC().Eth().SendRawTransaction(data)
	assert.NoError(t, err)
	assert.NotNil(t, hash)

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	receipt, err := srv.WaitForReceipt(ctx, hash)
	assert.NoError(t, err)
	assert.NotNil(t, receipt)

	// Check validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := srv.WaitForIBFTSnapshot(ctx, receipt.BlockNumber)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator := findValidatorByAddress(snapshot.Validators, stakerAddr.String())
	assert.Len(t, snapshot.Validators, numGenesisValidators+1)
	assert.NotNil(t, validator, "account should have joined to validator set, but doesn't belong to")
}

func TestPoS_Unstake(t *testing.T) {
	signer := &crypto.FrontierSigner{}
	unstakingContractAddr := types.StringToAddress("1002")

	// The last genesis validator will leave from validator set by unstaking
	numGenesisValidators := IBFTMinNodes + 1
	ibftManager := framework.NewIBFTServersManager(t, numGenesisValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		// Premine to send unstake transaction
		config.PremineValidatorBalance(framework.EthToWei(1), framework.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)
	srv := ibftManager.GetServer(0)

	// Get key of last node
	unstakerSrv := ibftManager.GetServer(IBFTMinNodes)
	unstakerKey, err := unstakerSrv.PrivateKey()
	assert.NoError(t, err)
	unstakerAddr := crypto.PubKeyToAddress(&unstakerKey.PublicKey)

	// Check the validator is in validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := srv.WaitForIBFTSnapshot(ctx, 0)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator := findValidatorByAddress(snapshot.Validators, unstakerAddr.String())
	assert.NotNil(t, validator, "account should be genesis validator, but isn't")

	// Send transaction to unstake
	txn := &types.Transaction{
		From:     unstakerAddr,
		To:       &unstakingContractAddr,
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    framework.EthToWei(0),
		Nonce:    0,
	}
	txn, err = signer.SignTx(txn, unstakerKey)
	assert.NoError(t, err)

	data := txn.MarshalRLP()
	hash, err := srv.JSONRPC().Eth().SendRawTransaction(data)
	assert.NoError(t, err)
	assert.NotNil(t, hash)

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	receipt, err := srv.WaitForReceipt(ctx, hash)
	assert.NoError(t, err)
	assert.NotNil(t, receipt)

	// Check validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err = srv.WaitForIBFTSnapshot(ctx, receipt.BlockNumber)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator = findValidatorByAddress(snapshot.Validators, unstakerAddr.String())
	assert.Nil(t, validator, "account should have left from validator set, but still belongs to")
	assert.Len(t, snapshot.Validators, numGenesisValidators-1)
}
