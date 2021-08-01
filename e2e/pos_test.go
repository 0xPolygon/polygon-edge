package e2e

import (
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/state/runtime/system"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
)

func findValidatorByAddress(validators []*proto.Snapshot_Validator, addr string) *proto.Snapshot_Validator {
	for _, v := range validators {
		if v.Address == addr {
			return v
		}
	}
	return nil
}

func findValidatorInSet(
	t *testing.T,
	srv *framework.TestServer,
	address types.Address,
	blockNumber uint64,
) (*proto.Snapshot_Validator, *proto.Snapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := srv.WaitForIBFTSnapshot(ctx, blockNumber, 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	return findValidatorByAddress(snapshot.Validators, address.String()), snapshot
}

func TestPoS_Stake(t *testing.T) {
	stakerKey, stakerAddr := framework.GenerateKeyAndAddr(t)
	stakingContractAddr := types.StringToAddress(system.StakingAddress)

	numGenesisValidators := IBFTMinNodes
	ibftManager := framework.NewIBFTServersManager(t, numGenesisValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.PremineWithStake(stakerAddr, tests.EthToWei(10), tests.EthToWei(10))
		config.PremineValidatorBalance(big.NewInt(0), tests.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)

	// Stake Balance
	txn := &framework.PreparedTransaction{
		From:     stakerAddr,
		To:       &stakingContractAddr,
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    tests.EthToWei(1),
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := srv.SendRawTx(ctx, txn, stakerKey)
	assert.NoError(t, err)

	// Check validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := srv.WaitForIBFTSnapshot(ctx, receipt.BlockNumber, 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator := findValidatorByAddress(snapshot.Validators, stakerAddr.String())
	assert.Len(t, snapshot.Validators, numGenesisValidators+1)
	assert.NotNil(t, validator, "expected staker to join the validator set")
}

func TestPoS_Unstake(t *testing.T) {
	unstakingContractAddr := types.StringToAddress(system.UnstakingAddress)

	// The last genesis validator will leave from validator set by unstaking
	numGenesisValidators := IBFTMinNodes + 1
	ibftManager := framework.NewIBFTServersManager(t, numGenesisValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		// Premine to send unstake transaction
		config.PremineValidatorBalance(tests.EthToWei(1), tests.EthToWei(10))
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)
	srv := ibftManager.GetServer(0)

	// Get key of last node
	unstakerSrv := ibftManager.GetServer(IBFTMinNodes)
	unstakerKey, err := unstakerSrv.Config.PrivateKey()
	assert.NoError(t, err)
	unstakerAddr := crypto.PubKeyToAddress(&unstakerKey.PublicKey)

	// Check the validator is in validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err := srv.WaitForIBFTSnapshot(ctx, 0, 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator := findValidatorByAddress(snapshot.Validators, unstakerAddr.String())
	assert.NotNil(t, validator, "account should be genesis validator, but isn't")

	// Send transaction to unstake
	txn := &framework.PreparedTransaction{
		From:     unstakerAddr,
		To:       &unstakingContractAddr,
		GasPrice: big.NewInt(10000),
		Gas:      1000000,
		Value:    tests.EthToWei(0),
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	receipt, err := srv.SendRawTx(ctx, txn, unstakerKey)
	assert.NoError(t, err)

	// Check validator set
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	snapshot, err = srv.WaitForIBFTSnapshot(ctx, receipt.BlockNumber, 5*time.Second)
	assert.NoError(t, err)
	assert.NotNil(t, snapshot)

	validator = findValidatorByAddress(snapshot.Validators, unstakerAddr.String())
	assert.Nil(t, validator, "account should have left from validator set, but still belongs to it")
	assert.Len(t, snapshot.Validators, numGenesisValidators-1)
}

func TestPoS_UnstakeAttack(t *testing.T) {
	// Predefined values
	unstakingContractAddr := types.StringToAddress(system.UnstakingAddress)
	numGenesisValidators := IBFTMinNodes + 1
	gasPrice := big.NewInt(10000)
	receiptArr := make([]*web3.Receipt, numGenesisValidators)
	defaultBalance := tests.EthToWei(10)

	// Set up the manager
	ibftManager := framework.NewIBFTServersManager(t, numGenesisValidators, IBFTDirPrefix, func(i int, config *framework.TestServerConfig) {
		config.PremineValidatorBalance(defaultBalance, defaultBalance)
		config.SetSeal(true)
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)
	srv := ibftManager.GetServer(0)

	// Get key of last node
	referenceSrv := ibftManager.GetServer(IBFTMinNodes)
	referenceKey, err := referenceSrv.Config.PrivateKey()
	assert.NoError(t, err)
	referenceAddr := crypto.PubKeyToAddress(&referenceKey.PublicKey)

	// Check the validator is in validator set
	validator, _ := findValidatorInSet(t, srv, referenceAddr, 0)
	if validator == nil {
		t.Fatalf("account should be genesis validator, but isn't")
	}

	// Set up the goroutine
	sendFunc := func(wg *sync.WaitGroup, t *testing.T, index int) {
		txn := &framework.PreparedTransaction{
			From:     referenceAddr,
			To:       &unstakingContractAddr,
			GasPrice: gasPrice,
			Gas:      1000000,
			Value:    tests.EthToWei(0),
		}

		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		receipt, err := srv.SendRawTx(ctx, txn, referenceKey)
		assert.NoError(t, err)
		assert.NotNil(t, receipt)

		receiptArr[index] = receipt

		wg.Done()
	}

	// Add pending unstake transactions to the txpool
	numTransactions := 5

	// Send out multiple unstake transactions
	var wg sync.WaitGroup

	// WaitGroup will wait for numTransactions go routines to finish
	wg.Add(numTransactions)
	for i := 0; i < numTransactions; i++ {
		go sendFunc(&wg, t, i)
	}

	wg.Wait()

	fee := new(big.Int).Mul(
		big.NewInt(int64(receiptArr[0].GasUsed)),
		big.NewInt(gasPrice.Int64()),
	)

	// Check to see if balances are valid

	// Returned stake
	expectedBalance := big.NewInt(0).Add(defaultBalance, defaultBalance)

	// Subtract tx fees
	expectedBalance = big.NewInt(0).Sub(
		expectedBalance,
		fee,
	)

	client := srv.JSONRPC()
	actualStakedBalance := getStakedBalance(referenceAddr, client, t)
	actualBalance := getAccountBalance(referenceAddr, client, t)

	// Make sure the account balance matches up
	assert.Equalf(t,
		expectedBalance.String(),
		actualBalance.String(),
		"Balance mismatch after unstake",
	)

	// Make sure the staked balance matches up
	assert.Equalf(t,
		big.NewInt(0).String(),
		actualStakedBalance.String(),
		"Staked balance mismatch after unstake",
	)

	// Check to see if the unstaker is still a validator
	validator, snapshot := findValidatorInSet(t, srv, referenceAddr, receiptArr[0].BlockNumber)
	assert.Nil(t, validator, "account should have left from validator set, but still belongs to it")
	assert.Len(t, snapshot.Validators, numGenesisValidators-1)
}
