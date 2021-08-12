package e2e

import (
	"context"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/consensus/ibft/proto"
	"github.com/0xPolygon/minimal/crypto"
	"github.com/0xPolygon/minimal/e2e/framework"
	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/state/runtime/system"
	txpoolOp "github.com/0xPolygon/minimal/txpool/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
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

func TestPoS_Stake(t *testing.T) {
	stakerKey, stakerAddr := tests.GenerateKeyAndAddr(t)
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

func waitForBlock(t *testing.T, srv *framework.TestServer, expectedBlocks int, index int) int64 {
	systemClient := srv.Operator()
	ctx, cancelFn := context.WithCancel(context.Background())
	stream, err := systemClient.Subscribe(ctx, &empty.Empty{})
	if err != nil {
		cancelFn()
		t.Fatalf("Unable to subscribe to blockchain events")
	}

	evnt, err := stream.Recv()
	if err == io.EOF {
		t.Fatalf("Invalid stream close")
	}
	if err != nil {
		t.Fatalf("Unable to read blockchain event")
	}

	if len(evnt.Added) != expectedBlocks {
		t.Fatalf("Invalid number of blocks added")
	}

	cancelFn()

	return evnt.Added[index].Number
}

func TestPoS_UnstakeExploit(t *testing.T) {
	// Predefined values
	unstakingContractAddr := types.StringToAddress(system.UnstakingAddress)
	stakingContractAddr := types.StringToAddress(system.StakingAddress)
	gasPrice := big.NewInt(10000)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := tests.EthToWei(10)

	devInterval := 10

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.PremineWithStake(senderAddr, defaultBalance, defaultBalance)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	previousAccountBalance := getAccountBalance(senderAddr, client, t)

	// Required default values
	numTransactions := 5
	signer := crypto.NewEIP155Signer(100)
	currentNonce := 0

	// TxPool client
	clt := srv.TxnPoolOperator()

	generateTx := func() *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    uint64(currentNonce),
			From:     types.ZeroAddress,
			To:       &unstakingContractAddr,
			GasPrice: gasPrice,
			Gas:      1000000,
			Value:    big.NewInt(0),
			V:        1, // it is necessary to encode in rlp
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	// Test scenario:
	// User has 10 ETH staked and a balance of 10 ETH
	// Unstake -> Unstake -> Unstake -> Unstake...
	// The code below tests numTransactions cycles of Unstake
	// Expected result for tests: Staked: 0 ETH; Balance: ~20 ETH

	zeroEth := tests.EthToWei(0)
	for i := 0; i < numTransactions; i++ {
		var msg *txpoolOp.AddTxnReq
		unstakeTxn := generateTx()

		msg = &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: unstakeTxn.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		}

		_, addErr := clt.AddTxn(context.Background(), msg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}
	}

	// Set up the blockchain listener to catch the added block event
	blockNum := waitForBlock(t, srv, 1, 0)

	block, blockErr := client.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), true)
	if blockErr != nil {
		t.Fatalf("Unable to fetch block")
	}

	// Find how much the account paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(gasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualStakedBalance := getStakedBalance(senderAddr, client, t)
	actualAccountBalance := getAccountBalance(senderAddr, client, t)
	actualStakingAddrBalance := getAccountBalance(stakingContractAddr, client, t)

	// Make sure the balances match up

	// expBalance = previousAccountBalance + stakeRefund - block fees
	expBalance := big.NewInt(0).Sub(big.NewInt(0).Add(previousAccountBalance, defaultBalance), paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after unstake exploit",
	)

	assert.Equalf(t,
		zeroEth.String(),
		actualStakedBalance.String(),
		"Staked balance mismatch after unstake exploit",
	)

	assert.Equalf(t,
		zeroEth.String(),
		actualStakingAddrBalance.String(),
		"Staked address balance mismatch after unstake exploit",
	)
}

func TestPoS_StakeUnstakeExploit(t *testing.T) {
	// Predefined values
	unstakingContractAddr := types.StringToAddress(system.UnstakingAddress)
	stakingContractAddr := types.StringToAddress(system.StakingAddress)
	gasPrice := big.NewInt(10000)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := tests.EthToWei(10)
	initialStakingAddrBalance := tests.EthToWei(1000)

	devInterval := 10

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.PremineWithStake(senderAddr, defaultBalance, defaultBalance)
		// After this callback the staking contract address will have a balance of 1010 ETH
		config.Premine(stakingContractAddr, initialStakingAddrBalance)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	// Required default values
	numTransactions := 6
	signer := crypto.NewEIP155Signer(100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(value *big.Int, to types.Address) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    uint64(currentNonce),
			From:     types.ZeroAddress,
			To:       &to,
			GasPrice: gasPrice,
			Gas:      1000000,
			Value:    value,
			V:        1, // it is necessary to encode in rlp
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	// Test scenario:
	// User has 10 ETH staked and a balance of 10 ETH
	// Unstake -> Stake 1 ETH -> Unstake -> Stake 1 ETH...
	// The code below tests (numTransactions / 2) cycles of Unstake -> Stake 1 ETH
	// Expected result for tests: Staked: 1 ETH; Balance: ~19 ETH

	oneEth := tests.EthToWei(1)
	zeroEth := tests.EthToWei(0)
	for i := 0; i < numTransactions; i++ {
		var msg *txpoolOp.AddTxnReq
		if i%2 == 0 {
			unstakeTxn := generateTx(zeroEth, unstakingContractAddr)
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: unstakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		} else {
			stakeTxn := generateTx(oneEth, stakingContractAddr)
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: stakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		}

		_, addErr := txpoolClient.AddTxn(context.Background(), msg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}
	}

	// Set up the blockchain listener to catch the added block event
	blockNum := waitForBlock(t, srv, 1, 0)

	block, blockErr := client.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), true)
	if blockErr != nil {
		t.Fatalf("Unable to fetch block")
	}

	// Find how much the account paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(gasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualStakedBalance := getStakedBalance(senderAddr, client, t)
	actualAccountBalance := getAccountBalance(senderAddr, client, t)
	actualStakingAddrBalance := getAccountBalance(stakingContractAddr, client, t)

	expStake := tests.EthToWei(1)

	// Make sure the staked balance matches up
	assert.Equalf(t,
		expStake.String(),
		actualStakedBalance.String(),
		"Staked balance mismatch after stake / unstake exploit",
	)

	assert.Equalf(t,
		(big.NewInt(0).Add(expStake, initialStakingAddrBalance)).String(),
		actualStakingAddrBalance.String(),
		"Staked address balance mismatch after stake / unstake exploit",
	)

	// Make sure the account balances match up

	// expBalance = previousAccountBalance + stakeRefund - 1 ETH - block fees
	expBalance := big.NewInt(0).Sub(big.NewInt(0).Add(defaultBalance, defaultBalance), oneEth)
	expBalance = big.NewInt(0).Sub(expBalance, paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after unstake exploit",
	)
}

func TestPoS_StakeUnstakeWithinSameBlock(t *testing.T) {
	// Predefined values
	unstakingContractAddr := types.StringToAddress(system.UnstakingAddress)
	stakingContractAddr := types.StringToAddress(system.StakingAddress)
	gasPrice := big.NewInt(10000)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := tests.EthToWei(5)
	initialStakingAddrBalance := tests.EthToWei(100)

	devInterval := 5

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(stakingContractAddr, initialStakingAddrBalance)
		config.Premine(senderAddr, defaultBalance)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	// Required default values
	signer := crypto.NewEIP155Signer(100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(value *big.Int, to types.Address) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    uint64(currentNonce),
			From:     types.ZeroAddress,
			To:       &to,
			GasPrice: gasPrice,
			Gas:      1000000,
			Value:    value,
			V:        1, // it is necessary to encode in rlp
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	// Test scenario:
	// User has 0 ETH staked and a balance of 5 ETH
	// Stake 2 ETH -> Unstake
	// Expected result for tests: Staked: 0 ETH; Balance: ~5 ETH

	oneEth := tests.EthToWei(1)
	zeroEth := tests.EthToWei(0)
	for i := 0; i < 2; i++ {
		var msg *txpoolOp.AddTxnReq
		if i%2 == 0 {
			stakeTxn := generateTx(oneEth, stakingContractAddr)
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: stakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		} else {
			unstakeTxn := generateTx(zeroEth, unstakingContractAddr)
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: unstakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		}

		_, addErr := txpoolClient.AddTxn(context.Background(), msg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}
	}

	// Set up the blockchain listener to catch the added block event
	blockNum := waitForBlock(t, srv, 1, 0)

	block, blockErr := client.Eth().GetBlockByNumber(web3.BlockNumber(blockNum), true)
	if blockErr != nil {
		t.Fatalf("Unable to fetch block")
	}

	// Find how much the account paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(gasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualStakedBalance := getStakedBalance(senderAddr, client, t)
	actualAccountBalance := getAccountBalance(senderAddr, client, t)
	actualStakingAddrBalance := getAccountBalance(stakingContractAddr, client, t)

	expStake := big.NewInt(0)

	// Make sure the staked balance matches up
	assert.Equalf(t,
		expStake.String(),
		actualStakedBalance.String(),
		"Staked balance mismatch after stake / unstake events",
	)

	assert.Equalf(t,
		initialStakingAddrBalance.String(),
		actualStakingAddrBalance.String(),
		"Staked address balance mismatch after stake / unstake events",
	)

	// Make sure the account balances match up

	// expBalance = previousAccountBalance - block fees
	expBalance := big.NewInt(0).Sub(defaultBalance, paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after stake / unstake events",
	)
}
