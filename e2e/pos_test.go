package e2e

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/contracts/staking"
	"github.com/0xPolygon/polygon-sdk/e2e/framework"
	"github.com/0xPolygon/polygon-sdk/helper/tests"
	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
)

// isInValidatorSet is a helper function for searching through the passed in set for a specific
// address
func isInValidatorSet(validatorSet []types.Address, searchValidator types.Address) bool {
	searchStr := searchValidator.String()
	for _, validator := range validatorSet {
		if validator.String() == searchStr {
			return true
		}
	}

	return false
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

			config.SetShowsLog(true)
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
	validatorSet, validatorSetErr := framework.GetValidatorSet(stakerAddr, client)
	if validatorSetErr != nil {
		t.Fatalf("Unable to fetch validator set, %v", validatorSetErr)
	}
	assert.NotNil(t, validatorSet)
	assert.Len(t, validatorSet, numGenesisValidators+1)

	assert.Truef(t,
		isInValidatorSet(validatorSet, stakerAddr),
		"expected staker to join the validator set",
	)

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
