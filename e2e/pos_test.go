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
	_, stakerAddr := tests.GenerateKeyAndAddr(t)
	defaultBalance := framework.EthToWei(100)

	numGenesisValidators := IBFTMinNodes
	ibftManager := framework.NewIBFTServersManager(
		t,
		numGenesisValidators,
		IBFTDirPrefix,
		func(i int, config *framework.TestServerConfig) {
			config.SetSeal(true)
			config.SetEpochSize(1)
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
	stakeError := framework.StakeAmount(stakerAddr, framework.EthToWei(5), client)
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
		big.NewInt(int64(numGenesisValidators+1)),
	)

	assert.Equal(t, expectedBalance.String(), scBalance.String())
}
