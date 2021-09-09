package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	"github.com/0xPolygon/polygon-sdk/crypto"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
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
	val := helper.DefaultStakedBalance
	bigDefaultStakedBalance, err := types.ParseUint256orHex(&val)
	if err != nil {
		t.Fatalf("unable to generate DefaultStatkedBalance, %v", err)
	}

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
	val := helper.DefaultStakedBalance
	bigDefaultStakedBalance, err := types.ParseUint256orHex(&val)
	if err != nil {
		t.Fatalf("unable to generate DefaultStatkedBalance, %v", err)
	}

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
