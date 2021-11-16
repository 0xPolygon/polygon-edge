package e2e

import (
	"context"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	txpoolOp "github.com/0xPolygon/polygon-sdk/txpool/proto"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/stretchr/testify/assert"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
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
	val := helper.DefaultStakedBalance
	bigDefaultStakedBalance, err := types.ParseUint256orHex(&val)
	if err != nil {
		t.Fatalf("unable to parse DefaultStakedBalance, %v", err)
	}

	return bigDefaultStakedBalance
}

// validateValidatorSet makes sure that the address is present / not present in the
// validator set, as well as if the validator set is of a certain size
func validateValidatorSet(
	address types.Address,
	client *jsonrpc.Client,
	t *testing.T,
	expectedExistence bool,
	expectedSize int,
) {
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
			config.SetSeal(true)
			config.SetEpochSize(2) // Need to leave room for the endblock
			config.PremineValidatorBalance(defaultBalance)
			config.Premine(stakerAddr, defaultBalance)
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
	validateValidatorSet(stakerAddr, client, t, true, numGenesisValidators+1)

	// Check the SC balance
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)

	scBalance := framework.GetAccountBalance(staking.AddrStakingContract, client, t)
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
		func(i int, config *framework.TestServerConfig) {
			// Premine to send unstake transaction
			config.SetSeal(true)
			config.SetEpochSize(2) // Need to leave room for the endblock
			config.PremineValidatorBalance(defaultBalance)
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
	validateValidatorSet(unstakerAddr, client, t, true, numGenesisValidators)

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
	validateValidatorSet(unstakerAddr, client, t, false, numGenesisValidators-1)

	// Check the SC balance
	bigDefaultStakedBalance := getBigDefaultStakedBalance(t)

	scBalance := framework.GetAccountBalance(staking.AddrStakingContract, client, t)
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

	// Check the account balance
	fee := new(big.Int).Mul(
		big.NewInt(int64(receipt.GasUsed)),
		big.NewInt(framework.DefaultGasPrice),
	)

	accountBalance := framework.GetAccountBalance(unstakerAddr, client, t)
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
	bigGasPrice := big.NewInt(framework.DefaultGasPrice)

	devInterval := 5 // s

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetDevStakingAddresses([]types.Address{senderAddr})
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	previousAccountBalance := framework.GetAccountBalance(senderAddr, client, t)

	// Check if the stake is present on the SC
	actualStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	assert.Equalf(t,
		bigDefaultStakedBalance.String(),
		actualStakingSCBalance.String(),
		"Staked address balance mismatch before unstake exploit",
	)

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
			To:       &stakingContractAddr,
			GasPrice: bigGasPrice,
			Gas:      framework.DefaultGasLimit,
			Value:    big.NewInt(0),
			V:        []byte{1}, // it is necessary to encode in rlp,
			Input:    framework.MethodSig("unstake"),
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	zeroEth := framework.EthToWei(0)
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
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(senderAddr, client, t)
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
		zeroEth.String(),
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
	bigGasPrice := big.NewInt(framework.DefaultGasPrice)

	senderKey, senderAddr := tests.GenerateKeyAndAddr(t)
	numDummyStakers := 100

	devInterval := 5 // s

	// Set up the test server
	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
		config.SetConsensus(framework.ConsensusDev)
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetBlockLimit(blockGasLimit)
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
	signer := crypto.NewEIP155Signer(100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(value *big.Int, methodName string) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    uint64(currentNonce),
			From:     types.ZeroAddress,
			To:       &stakingContractAddr,
			GasPrice: bigGasPrice,
			Gas:      framework.DefaultGasLimit,
			Value:    value,
			V:        []byte{1}, // it is necessary to encode in rlp
			Input:    framework.MethodSig(methodName),
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	oneEth := framework.EthToWei(1)
	zeroEth := framework.EthToWei(0)
	for i := 0; i < numTransactions; i++ {
		var msg *txpoolOp.AddTxnReq
		if i%2 == 0 {
			unstakeTxn := generateTx(zeroEth, "unstake")
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: unstakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		} else {
			stakeTxn := generateTx(oneEth, "stake")
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
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(senderAddr, client, t)
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

	// Make sure the account balances match up

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
// Expected result for tests: Staked: 0 ETH; Balance: ~100 ETH
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
		config.SetSeal(true)
		config.SetDevInterval(devInterval)
		config.Premine(senderAddr, defaultBalance)
		config.SetBlockLimit(blockGasLimit)
		config.SetDevStakingAddresses(generateStakingAddresses(numDummyStakers))
	})
	srv := srvs[0]
	client := srv.JSONRPC()

	initialStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	// Required default values
	signer := crypto.NewEIP155Signer(100)
	currentNonce := 0

	// TxPool client
	txpoolClient := srv.TxnPoolOperator()

	generateTx := func(value *big.Int, methodName string) *types.Transaction {
		signedTx, signErr := signer.SignTx(&types.Transaction{
			Nonce:    uint64(currentNonce),
			From:     types.ZeroAddress,
			To:       &stakingContractAddr,
			GasPrice: bigGasPrice,
			Gas:      framework.DefaultGasLimit,
			Value:    value,
			V:        []byte{1}, // it is necessary to encode in rlp
			Input:    framework.MethodSig(methodName),
		}, senderKey)

		if signErr != nil {
			t.Fatalf("Unable to sign transaction, %v", signErr)
		}

		currentNonce++

		return signedTx
	}

	oneEth := framework.EthToWei(1)
	zeroEth := framework.EthToWei(0)
	for i := 0; i < 2; i++ {
		var msg *txpoolOp.AddTxnReq
		if i%2 == 0 {
			stakeTxn := generateTx(oneEth, "stake")
			msg = &txpoolOp.AddTxnReq{
				Raw: &any.Any{
					Value: stakeTxn.MarshalRLP(),
				},
				From: types.ZeroAddress.String(),
			}
		} else {
			unstakeTxn := generateTx(zeroEth, "unstake")
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
	paidFee := big.NewInt(0).Mul(bigGasPrice, big.NewInt(int64(block.GasUsed)))

	// Check the balances
	actualAccountBalance := framework.GetAccountBalance(senderAddr, client, t)
	actualStakingSCBalance, fetchError := framework.GetStakedAmount(senderAddr, client)
	if fetchError != nil {
		t.Fatalf("Unable to fetch staking SC balance, %v", fetchError)
	}

	assert.Equalf(t,
		initialStakingSCBalance.String(),
		actualStakingSCBalance.String(),
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
