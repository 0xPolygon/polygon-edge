package e2e

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"

	"github.com/0xPolygon/polygon-edge/chain"
	ibftOp "github.com/0xPolygon/polygon-edge/consensus/ibft/proto"
	"github.com/0xPolygon/polygon-edge/contracts/staking"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/e2e/framework"
	"github.com/0xPolygon/polygon-edge/helper/common"
	stakingHelper "github.com/0xPolygon/polygon-edge/helper/staking"
	"github.com/0xPolygon/polygon-edge/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/golang/protobuf/ptypes/any"
)

// foundInValidatorSet is a helper function for searching through the passed in set for a specific
// address
func foundInValidatorSet(validatorSet []types.Address, searchValidator types.Address) bool {
	searchStr := searchValidator.String()
	for _, validator := range validatorSet {
		if validator.String() == searchStr {
			return true
		}
	}

	return false
}

// getBigDefaultStakedBalance returns the default staked balance as a *big.Int
func getBigDefaultStakedBalance(t *testing.T) *big.Int {
	t.Helper()

	val := stakingHelper.DefaultStakedBalance
	bigDefaultStakedBalance, err := common.ParseUint256orHex(&val)

	if err != nil {
		t.Fatalf("unable to parse DefaultStakedBalance, %v", err)
	}

	return bigDefaultStakedBalance
}

// validateValidatorSet makes sure that the address is present / not present in the
// validator set, as well as if the validator set is of a certain size
func validateValidatorSet(
	t *testing.T,
	address types.Address,
	client *jsonrpc.Client,
	expectedExistence bool,
	expectedSize int,
) {
	t.Helper()

	validatorSet, validatorSetErr := framework.GetValidatorSet(address, client)
	if validatorSetErr != nil {
		t.Fatalf("Unable to fetch validator set, %v", validatorSetErr)
	}

	assert.NotNil(t, validatorSet)
	assert.Len(t, validatorSet, expectedSize)

	if expectedExistence {
		assert.Truef(
			t,
			foundInValidatorSet(validatorSet, address),
			"expected address to be present in the validator set",
		)
	} else {
		assert.Falsef(t,
			foundInValidatorSet(validatorSet, address),
			"expected address to not be present in the validator set",
		)
	}
}

func TestPoS_ValidatorBoundaries(t *testing.T) {
	accounts := []struct {
		key     *ecdsa.PrivateKey
		address types.Address
	}{}
	stakeAmount := framework.EthToWei(1)
	numGenesisValidators := IBFTMinNodes
	minValidatorCount := uint64(1)
	maxValidatorCount := uint64(numGenesisValidators + 1)
	numNewStakers := 2

	for i := 0; i < numNewStakers; i++ {
		k, a := tests.GenerateKeyAndAddr(t)

		accounts = append(accounts, struct {
			key     *ecdsa.PrivateKey
			address types.Address
		}{
			key:     k,
			address: a,
		})
	}

	defaultBalance := framework.EthToWei(100)
	ibftManager := framework.NewIBFTServersManager(
		t,
		numGenesisValidators,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.SetEpochSize(2)
			config.PremineValidatorBalance(defaultBalance)
			for j := 0; j < numNewStakers; j++ {
				config.Premine(accounts[j].address, defaultBalance)
			}
			config.SetIBFTPoS(true)
			config.SetMinValidatorCount(minValidatorCount)
			config.SetMaxValidatorCount(maxValidatorCount)
		})

	t.Cleanup(func() {
		ibftManager.StopServers()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)

	client := srv.JSONRPC()

	testCases := []struct {
		name              string
		address           types.Address
		key               *ecdsa.PrivateKey
		expectedExistence bool
		expectedSize      int
	}{
		{
			name:              "Can add a 5th validator",
			address:           accounts[0].address,
			key:               accounts[0].key,
			expectedExistence: true,
			expectedSize:      numGenesisValidators + 1,
		},
		{
			name:              "Can not add a 6th validator",
			address:           accounts[1].address,
			key:               accounts[1].key,
			expectedExistence: false,
			expectedSize:      numGenesisValidators + 1,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := framework.StakeAmount(tt.address, tt.key, stakeAmount, srv)
			assert.NoError(t, err)
			validateValidatorSet(t, tt.address, client, tt.expectedExistence, tt.expectedSize)
		})
	}
}

func TestPoS_Stake(t *testing.T) {
	stakerKey, stakerAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := framework.EthToWei(100)
	stakeAmount := framework.EthToWei(5)

	numGenesisValidators := IBFTMinNodes
	ibftManager := framework.NewIBFTServersManager(
		t,
		numGenesisValidators,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.SetEpochSize(2) // Need to leave room for the endblock
			config.PremineValidatorBalance(defaultBalance)
			config.Premine(stakerAddr, defaultBalance)
			config.SetIBFTPoS(true)
		})

	t.Cleanup(func() {
		ibftManager.StopServers()
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)

	srv := ibftManager.GetServer(0)

	client := srv.JSONRPC()

	// Stake Balance
	stakeError := framework.StakeAmount(
		stakerAddr,
		stakerKey,
		stakeAmount,
		srv,
	)
	if stakeError != nil {
		t.Fatalf("Unable to stake amount, %v", stakeError)
	}

	// Check validator set
	validateValidatorSet(t, stakerAddr, client, true, numGenesisValidators+1)

	// Check the SC balance
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)

	scBalance := framework.GetAccountBalance(t, staking.AddrStakingContract, client)
	expectedBalance := big.NewInt(0).Mul(
		bigDefaultStakedBalance,
		big.NewInt(int64(numGenesisValidators)),
	)
	expectedBalance.Add(expectedBalance, stakeAmount)

	assert.Equal(t, expectedBalance.String(), scBalance.String())

	stakedAmount, stakedAmountErr := framework.GetStakedAmount(stakerAddr, client)
	if stakedAmountErr != nil {
		t.Fatalf("Unable to get staked amount, %v", stakedAmountErr)
	}

	assert.Equal(t, expectedBalance.String(), stakedAmount.String())
}

func TestPoS_Unstake(t *testing.T) {
	stakingContractAddr := staking.AddrStakingContract
	defaultBalance := framework.EthToWei(100)

	// The last genesis validator will leave from validator set by unstaking
	numGenesisValidators := IBFTMinNodes + 1
	ibftManager := framework.NewIBFTServersManager(
		t,
		numGenesisValidators,
		IBFTDirPrefix,
		func(_ int, config *framework.TestServerConfig) {
			// Premine to send unstake transaction
			config.SetEpochSize(2) // Need to leave room for the endblock
			config.PremineValidatorBalance(defaultBalance)
			config.SetIBFTPoS(true)
		})

	t.Cleanup(func() {
		ibftManager.StopServers()
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

	client := srv.JSONRPC()

	// Check the validator is in validator set
	validateValidatorSet(t, unstakerAddr, client, true, numGenesisValidators)

	// Send transaction to unstake
	receipt, unstakeError := framework.UnstakeAmount(
		unstakerAddr,
		unstakerKey,
		srv,
	)
	if unstakeError != nil {
		t.Fatalf("Unable to unstake amount, %v", unstakeError)
	}

	// Check validator set
	validateValidatorSet(t, unstakerAddr, client, false, numGenesisValidators-1)

	// Check the SC balance
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)

	scBalance := framework.GetAccountBalance(t, staking.AddrStakingContract, client)
	expectedBalance := big.NewInt(0).Mul(
		bigDefaultStakedBalance,
		big.NewInt(int64(numGenesisValidators)),
	)
	expectedBalance.Sub(expectedBalance, bigDefaultStakedBalance)

	assert.Equal(t, expectedBalance.String(), scBalance.String())

	stakedAmount, stakedAmountErr := framework.GetStakedAmount(stakingContractAddr, client)
	if stakedAmountErr != nil {
		t.Fatalf("Unable to get staked amount, %v", stakedAmountErr)
	}

	assert.Equal(t, expectedBalance.String(), stakedAmount.String())

	// Check the address balance
	fee := new(big.Int).Mul(
		big.NewInt(int64(receipt.GasUsed)),
		ethgo.Gwei(1),
	)

	accountBalance := framework.GetAccountBalance(t, unstakerAddr, client)
	expectedAccountBalance := big.NewInt(0).Add(defaultBalance, bigDefaultStakedBalance)
	expectedAccountBalance.Sub(expectedAccountBalance, fee)

	assert.Equal(t, expectedAccountBalance.String(), accountBalance.String())
}

// Test scenario:
// User has 10 ETH staked and a balance of 10 ETH
// Unstake -> Unstake -> Unstake -> Unstake...
// The code below tests numTransactions cycles of Unstake
// Expected result for tests: Staked: 0 ETH; Balance: ~20 ETH
func TestPoS_UnstakeExploit(t *testing.T) {
	// Predefined values
	stakingContractAddr := staking.AddrStakingContract

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)
	defaultBalance := framework.EthToWei(100)
	bigGasPrice := big.NewInt(1000000000)

	devInterval := 5 // s
	numDummyValidators := 5

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetDevStakingAddresses(append(generateStakingAddresses(numDummyValidators), senderAddr))
		config.SetIBFTPoS(true)
		config.SetBlockLimit(5000000000)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	previousAccountBalance := framework.GetAccountBalance(t, senderAddr, client)

	// Check if the stake is present on the SC
	actualStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	assert.Equalf(t,
		big.NewInt(0).Mul(bigDefaultStakedBalance, big.NewInt(int64(numDummyValidators+1))).String(),
		actualStakingSCBalance.String(),
		"Staked address balance mismatch before unstake exploit",
	)

	// Required default values
	numTransactions := 5
	signer := crypto.NewSigner(chain.AllForksEnabled.At(0), 100)
	currentNonce := 0

	// TxPool client
	clt := srv.TxnPoolOperator()

	generateTx := func(i int) *types.Transaction {
		unsignedTx := &types.Transaction{
			Nonce: uint64(currentNonce),
			From:  types.ZeroAddress,
			To:    &stakingContractAddr,
			Gas:   framework.DefaultGasLimit,
			Value: big.NewInt(0),
			V:     big.NewInt(1), // it is necessary to encode in rlp,
			Input: framework.MethodSig("unstake"),
		}

		// Just make very second transaction with dynamic gas fee
		if i%2 == 0 {
			unsignedTx.Type = types.DynamicFeeTx
			unsignedTx.GasFeeCap = bigGasPrice
			unsignedTx.GasTipCap = bigGasPrice
		} else {
			unsignedTx.Type = types.LegacyTx
			unsignedTx.GasPrice = bigGasPrice
		}

		signedTx, err := signer.SignTx(unsignedTx, senderKey)
		require.NoError(t, err, "Unable to sign transaction")

		currentNonce++

		return signedTx
	}

	txHashes := make([]ethgo.Hash, 0)

	for i := 0; i < numTransactions; i++ {
		var msg *txpoolOp.AddTxnReq

		unstakeTxn := generateTx(i)

		msg = &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: unstakeTxn.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		}

		addCtx, addCtxCn := context.WithTimeout(context.Background(), framework.DefaultTimeout)

		addResp, addErr := clt.AddTxn(addCtx, msg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}

		txHashes = append(txHashes, ethgo.HexToHash(addResp.TxHash))

		addCtxCn()
	}

	// Wait for the transactions to go through
	totalGasUsed := srv.GetGasTotal(txHashes)

	// Find how much the address paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(totalGasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(t, senderAddr, client)
	actualStakingSCBalance, fetchError = framework.GetStakedAmount(senderAddr, client)

	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	// Make sure the balances match up

	// expBalance = previousAccountBalance + stakeRefund - block fees
	expBalance := big.NewInt(0).Sub(big.NewInt(0).Add(previousAccountBalance, bigDefaultStakedBalance), paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after unstake exploit",
	)

	assert.Equalf(t,
		big.NewInt(0).Mul(bigDefaultStakedBalance, big.NewInt(int64(numDummyValidators))).String(),
		actualStakingSCBalance.String(),
		"Staked address balance mismatch after unstake exploit",
	)
}

// generateStakingAddresses is a helper method for generating dummy staking addresses
func generateStakingAddresses(numAddresses int) []types.Address {
	result := make([]types.Address, numAddresses)

	for i := 0; i < numAddresses; i++ {
		result[i] = types.StringToAddress(strconv.Itoa(i + 100))
	}

	return result
}

// Test scenario:
// User has 10 ETH staked and a balance of 100 ETH
// Unstake -> Stake 1 ETH -> Unstake -> Stake 1 ETH...
// The code below tests (numTransactions / 2) cycles of Unstake -> Stake 1 ETH
// Expected result for tests: Staked: 1 ETH; Balance: ~119 ETH
func TestPoS_StakeUnstakeExploit(t *testing.T) {
	// Predefined values
	var blockGasLimit uint64 = 5000000000

	stakingContractAddr := staking.AddrStakingContract
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)
	defaultBalance := framework.EthToWei(100)
	bigGasPrice := big.NewInt(1000000000)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	numDummyStakers := 100

	devInterval := 5 // s

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetBlockLimit(blockGasLimit)
		config.SetIBFTPoS(true)
		// This call will add numDummyStakers + 1 staking address to the staking SC.
		// This is done in order to pump the stakedAmount value on the staking SC
		config.SetDevStakingAddresses(append(generateStakingAddresses(numDummyStakers), senderAddr))
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	initialStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	assert.Equalf(t,
		(big.NewInt(0).Mul(big.NewInt(int64(numDummyStakers+1)), bigDefaultStakedBalance)).String(),
		initialStakingSCBalance.String(),
		"Staked address balance mismatch before stake / unstake exploit",
	)

	// Required default values
	numTransactions := 6
	signer := crypto.NewSigner(chain.AllForksEnabled.At(0), 100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(i int, value *big.Int, methodName string) *types.Transaction {
		unsignedTx := &types.Transaction{
			Nonce: uint64(currentNonce),
			From:  types.ZeroAddress,
			To:    &stakingContractAddr,
			Gas:   framework.DefaultGasLimit,
			Value: value,
			V:     big.NewInt(1), // it is necessary to encode in rlp
			Input: framework.MethodSig(methodName),
		}

		// Just make very second transaction with dynamic gas fee
		if i%2 == 0 {
			unsignedTx.Type = types.DynamicFeeTx
			unsignedTx.GasFeeCap = bigGasPrice
			unsignedTx.GasTipCap = bigGasPrice
		} else {
			unsignedTx.Type = types.LegacyTx
			unsignedTx.GasPrice = bigGasPrice
		}

		signedTx, err := signer.SignTx(unsignedTx, senderKey)
		require.NoError(t, err, "Unable to sign transaction")

		currentNonce++

		return signedTx
	}

	oneEth := framework.EthToWei(1)
	zeroEth := framework.EthToWei(0)
	txHashes := make([]ethgo.Hash, 0)

	for i := 0; i < numTransactions; i++ {
		var msg *txpoolOp.AddTxnReq

		if i%2 == 0 {
			unstakeTxn := generateTx(i, zeroEth, "unstake")
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: unstakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		} else {
			stakeTxn := generateTx(i, oneEth, "stake")
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: stakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		}

		addResp, addErr := txpoolClient.AddTxn(context.Background(), msg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}

		txHashes = append(txHashes, ethgo.HexToHash(addResp.TxHash))
	}

	// Set up the blockchain listener to catch the added block event
	totalGasUsed := srv.GetGasTotal(txHashes)

	// Find how much the address paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(totalGasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(t, senderAddr, client)
	actualStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)

	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	expStake := big.NewInt(0).Mul(big.NewInt(int64(numDummyStakers)), bigDefaultStakedBalance)
	expStake.Add(expStake, oneEth)

	assert.Equalf(t,
		expStake.String(),
		actualStakingSCBalance.String(),
		"Staked address balance mismatch after stake / unstake exploit",
	)

	// Make sure the address balances match up

	// expBalance = previousAccountBalance + stakeRefund - 1 ETH - block fees
	expBalance := big.NewInt(0).Sub(big.NewInt(0).Add(defaultBalance, bigDefaultStakedBalance), oneEth)
	expBalance = big.NewInt(0).Sub(expBalance, paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after stake / unstake exploit",
	)
}

// Test scenario:
// User has 0 ETH staked and a balance of 100 ETH
// Stake 2 ETH -> Unstake
// Expected result for tests: Staked: 0 ETH; Balance: ~100 ETH; not a validator
func TestPoS_StakeUnstakeWithinSameBlock(t *testing.T) {
	// Predefined values
	var blockGasLimit uint64 = 5000000000

	stakingContractAddr := staking.AddrStakingContract
	defaultBalance := framework.EthToWei(100)
	bigGasPrice := big.NewInt(framework.DefaultGasPrice)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	numDummyStakers := 10

	devInterval := 5 // s

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetBlockLimit(blockGasLimit)
		config.SetDevStakingAddresses(generateStakingAddresses(numDummyStakers))
		config.SetIBFTPoS(true)
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	initialStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	// Required default values
	signer := crypto.NewSigner(chain.AllForksEnabled.At(0), 100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(dynamicTx bool, value *big.Int, methodName string) *types.Transaction {
		unsignedTx := &types.Transaction{
			Nonce: uint64(currentNonce),
			From:  types.ZeroAddress,
			To:    &stakingContractAddr,
			Gas:   framework.DefaultGasLimit,
			Value: value,
			V:     big.NewInt(1), // it is necessary to encode in rlp
			Input: framework.MethodSig(methodName),
		}

		if dynamicTx {
			unsignedTx.Type = types.DynamicFeeTx
			unsignedTx.GasFeeCap = bigGasPrice
			unsignedTx.GasTipCap = bigGasPrice
		} else {
			unsignedTx.Type = types.LegacyTx
			unsignedTx.GasPrice = bigGasPrice
		}

		signedTx, err := signer.SignTx(unsignedTx, senderKey)
		require.NoError(t, err, "Unable to signatransaction")

		currentNonce++

		return signedTx
	}

	zeroEth := framework.EthToWei(0)
	txHashes := make([]ethgo.Hash, 0)

	// addTxn is a helper method for generating and adding a transaction
	// through the operator command
	addTxn := func(dynamicTx bool, value *big.Int, methodName string) {
		txn := generateTx(dynamicTx, value, methodName)
		txnMsg := &txpoolOp.AddTxnReq{
			Raw: &any.Any{
				Value: txn.MarshalRLP(),
			},
			From: types.ZeroAddress.String(),
		}

		addResp, addErr := txpoolClient.AddTxn(context.Background(), txnMsg)
		if addErr != nil {
			t.Fatalf("Unable to add txn, %v", addErr)
		}

		txHashes = append(txHashes, ethgo.HexToHash(addResp.TxHash))
	}

	// Stake transaction
	addTxn(false, oneEth, "stake")

	// Unstake transaction
	addTxn(true, zeroEth, "unstake")

	// Wait for the transactions to go through
	totalGasUsed := srv.GetGasTotal(txHashes)

	// Find how much the address paid for all the transactions in this block
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(totalGasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(t, senderAddr, client)
	actualStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)

	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	assert.Equalf(t,
		initialStakingSCBalance.String(),
		actualStakingSCBalance.String(),
		"Staked address balance mismatch after stake / unstake events",
	)

	// Make sure the address balances match up

	// expBalance = previousAccountBalance - block fees
	expBalance := big.NewInt(0).Sub(defaultBalance, paidFee)

	assert.Equalf(t,
		expBalance.String(),
		actualAccountBalance.String(),
		"Account balance mismatch after stake / unstake events",
	)

	validateValidatorSet(t, senderAddr, client, false, numDummyStakers)
}

func getSnapshot(
	client ibftOp.IbftOperatorClient,
	blockNum uint64,
	ctx context.Context,
) (*ibftOp.Snapshot, error) {
	snapshot, snapshotErr := client.GetSnapshot(ctx, &ibftOp.SnapshotReq{
		Latest: false,
		Number: blockNum,
	})

	return snapshot, snapshotErr
}

func getNextEpochBlock(blockNum uint64, epochSize uint64) uint64 {
	if epochSize > blockNum {
		return epochSize
	}

	return epochSize*(blockNum/epochSize) + epochSize
}

func TestSnapshotUpdating(t *testing.T) {
	faucetKey, faucetAddr := tests.GenerateKeyAndAddr(t)

	defaultBalance := framework.EthToWei(1000)
	stakeAmount := framework.EthToWei(5)
	epochSize := uint64(5)

	numGenesisValidators := IBFTMinNodes
	numNonValidators := 2
	totalServers := numGenesisValidators + numNonValidators

	ibftManager := framework.NewIBFTServersManager(
		t,
		totalServers,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			if i < numGenesisValidators {
				// Only IBFTMinNodes should be validators
				config.PremineValidatorBalance(defaultBalance)
			} else {
				// Other nodes should not be in the validator set
				dirPrefix := "polygon-edge-non-validator-"
				config.SetIBFTDirPrefix(dirPrefix)
				config.SetIBFTDir(fmt.Sprintf("%s%d", dirPrefix, i))
			}

			config.SetEpochSize(epochSize)
			config.Premine(faucetAddr, defaultBalance)
			config.SetIBFTPoS(true)
		})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ibftManager.StartServers(ctx)
	firstValidator := ibftManager.GetServer(0)

	// Make sure the non-validator has funds before staking
	firstNonValidator := ibftManager.GetServer(IBFTMinNodes)
	firstNonValidatorKey, err := firstNonValidator.Config.PrivateKey()
	assert.NoError(t, err)

	firstNonValidatorAddr := crypto.PubKeyToAddress(&firstNonValidatorKey.PublicKey)

	sendCtx, sendWaitFn := context.WithTimeout(context.Background(), time.Second*30)
	defer sendWaitFn()

	receipt, transferErr := firstValidator.SendRawTx(
		sendCtx,
		&framework.PreparedTransaction{
			From:     faucetAddr,
			To:       &firstNonValidatorAddr,
			GasPrice: ethgo.Gwei(1),
			Gas:      1000000,
			Value:    framework.EthToWei(300),
		}, faucetKey)
	if transferErr != nil {
		t.Fatalf("Unable to transfer funds, %v", transferErr)
	}

	// Now that the non-validator has funds, they can stake
	stakeError := framework.StakeAmount(
		firstNonValidatorAddr,
		firstNonValidatorKey,
		stakeAmount,
		firstValidator,
	)

	if stakeError != nil {
		t.Fatalf("Unable to stake amount, %v", stakeError)
	}

	// Check validator set on the Staking Smart Contract
	validateValidatorSet(t, firstNonValidatorAddr, firstValidator.JSONRPC(), true, numGenesisValidators+1)

	// Find the nearest next epoch block
	nextEpoch := getNextEpochBlock(receipt.BlockNumber, epochSize) + epochSize

	servers := make([]*framework.TestServer, 0)
	for i := 0; i < totalServers; i++ {
		servers = append(servers, ibftManager.GetServer(i))
	}

	// Wait for all the nodes to reach the epoch block
	waitErrors := framework.WaitForServersToSeal(servers, nextEpoch+1)

	if len(waitErrors) != 0 {
		t.Fatalf("Unable to wait for all nodes to seal blocks, %v", waitErrors)
	}

	// Grab all the operators
	serverOperators := make([]ibftOp.IbftOperatorClient, totalServers)
	for i := 0; i < totalServers; i++ {
		serverOperators[i] = ibftManager.GetServer(i).IBFTOperator()
	}

	// isValidatorInSnapshot checks if a certain reference address
	// is among the validators for the specific snapshot
	isValidatorInSnapshot := func(
		client ibftOp.IbftOperatorClient,
		blockNumber uint64,
		referenceAddr types.Address,
	) bool {
		snapshotCtx, ctxCancelFn := context.WithTimeout(context.Background(), time.Second*5)
		snapshot, snapshotErr := getSnapshot(client, blockNumber, snapshotCtx)

		if snapshotErr != nil {
			t.Fatalf("Unable to fetch snapshot, %v", snapshotErr)
		}

		ctxCancelFn()

		for _, validator := range snapshot.Validators {
			if types.BytesToAddress(validator.Data) == referenceAddr {
				return true
			}
		}

		return false
	}

	// Make sure every node in the network has good snapshot upkeep
	for i := 0; i < totalServers; i++ {
		// Check the snapshot before the node became a validator
		assert.Falsef(
			t,
			isValidatorInSnapshot(serverOperators[i], nextEpoch-1, firstNonValidatorAddr),
			fmt.Sprintf(
				"Validator [%s] is in the snapshot validator list for block %d",
				firstNonValidatorAddr,
				nextEpoch-1,
			),
		)

		// Check the snapshot after the node became a validator
		assert.Truef(
			t,
			isValidatorInSnapshot(serverOperators[i], nextEpoch+1, firstNonValidatorAddr),
			fmt.Sprintf(
				"Validator [%s] is not in the snapshot validator list for block %d",
				firstNonValidatorAddr,
				nextEpoch+1,
			),
		)
	}
}
