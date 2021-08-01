package staking

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/helper/tests"
	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

type testStakeMapping struct {
	address types.Address
	stake   *big.Int
}

func instantiateStakingMap(mappings []testStakeMapping) {
	hub := GetStakingHub()

	hub.StakingMutex.Lock()
	for _, stakePair := range mappings {
		hub.StakingMap[stakePair.address] = stakePair.stake
	}
	hub.StakingMutex.Unlock()
}

func TestStakingHub_GetInstance(t *testing.T) {
	testTable := []struct {
		name         string
		numInstances int
	}{
		{
			"Valid instance",
			1,
		},
		{
			"Multiple instance calls",
			5,
		},
	}

	for indx, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			instances := make([]*StakingHub, testCase.numInstances)

			for i := 0; i < testCase.numInstances; i++ {
				instances[i] = GetStakingHub()

				assert.NotNilf(t, instances[i], "Staking hub instance invalid")
			}

			if indx == 1 && testCase.numInstances > 1 {
				// Check if the instances reference the same singleton
				dummyDir := "dummyDir"
				instances[0].SetWorkingDirectory(dummyDir)

				for i := 1; i < testCase.numInstances; i++ {
					assert.Equalf(t,
						instances[0].WorkingDirectory,
						instances[i].WorkingDirectory,
						"Staking hub instances not equal",
					)
				}
			}
		})
	}
}

func TestStakingHub_GetStakedBalance(t *testing.T) {
	validStakes := []struct {
		address      types.Address
		initialStake *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("123"),
			tests.EthToWei(50), // 50 ETH
		},

		// Valid account #2
		{
			types.StringToAddress("456"),
			tests.EthToWei(10), // 10 ETH
		},

		// Valid account #3
		{
			types.StringToAddress("789"),
			tests.EthToWei(5), // 5 ETH
		},
	}

	hub := GetStakingHub()

	// Instantiate the staking map
	for _, stakePair := range validStakes {
		hub.StakingMap[stakePair.address] = stakePair.initialStake
	}

	// Check the balances
	for _, stakePair := range validStakes {
		balance := hub.GetStakedBalance(stakePair.address)

		assert.Equalf(t, stakePair.initialStake, balance, "Staked balance doesn't match")
	}
}

func TestStakingHub_IncreaseStake(t *testing.T) {
	validStakes := []struct {
		address        types.Address
		initialStake   *big.Int
		increaseAmount *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("123"),
			tests.EthToWei(50), // 50 ETH
			tests.EthToWei(5),  // 5 ETH
		},

		// Valid account #2
		{
			types.StringToAddress("456"),
			tests.EthToWei(10), // 10 ETH
			tests.EthToWei(1),  // 1 ETH
		},

		// Valid account #3
		{
			types.StringToAddress("789"),
			tests.EthToWei(0),  // 0 ETH
			tests.EthToWei(10), // 10 ETH
		},
	}

	hub := GetStakingHub()
	// Instantiate the staking map
	var mappings []testStakeMapping
	for _, stakePair := range validStakes {
		mappings = append(mappings, testStakeMapping{
			address: stakePair.address,
			stake:   stakePair.initialStake,
		})
	}

	instantiateStakingMap(mappings)

	// Increase the stake
	for _, stakePair := range validStakes {
		expectedStake := big.NewInt(0).Add(stakePair.initialStake, stakePair.increaseAmount)

		hub.IncreaseStake(stakePair.address, stakePair.increaseAmount)

		assert.Equalf(t, expectedStake, hub.GetStakedBalance(stakePair.address), "Stake amount not valid")
	}
}

func TestStakingHub_DecreaseStake(t *testing.T) {
	validStakes := []struct {
		address        types.Address
		initialStake   *big.Int
		decreaseAmount *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("123"),
			tests.EthToWei(50), // 50 ETH
			tests.EthToWei(5),  // 5 ETH
		},

		// Valid account #2
		{
			types.StringToAddress("456"),
			tests.EthToWei(10), // 10 ETH
			tests.EthToWei(1),  // 1 ETH
		},

		// Invalid account #3
		{
			types.StringToAddress("789"),
			tests.EthToWei(0),  // 0 ETH
			tests.EthToWei(10), // 10 ETH
		},
	}

	hub := GetStakingHub()

	// Instantiate the staking map
	var mappings []testStakeMapping
	for _, stakePair := range validStakes {
		mappings = append(mappings, testStakeMapping{
			address: stakePair.address,
			stake:   stakePair.initialStake,
		})
	}
	instantiateStakingMap(mappings)

	// Decrease the stake
	for _, stakePair := range validStakes {
		var expectedStake *big.Int

		if stakePair.initialStake.Cmp(stakePair.decreaseAmount) < 0 {
			// Decrease should have no effect
			expectedStake = stakePair.initialStake
		} else {
			expectedStake = big.NewInt(0).Sub(stakePair.initialStake, stakePair.decreaseAmount)
		}

		hub.DecreaseStake(stakePair.address, stakePair.decreaseAmount)

		assert.Equalf(t, expectedStake, hub.GetStakedBalance(stakePair.address), "Stake amount not valid")
	}
}

func TestStakingHub_ResetStake(t *testing.T) {
	validStakes := []struct {
		address      types.Address
		initialStake *big.Int
	}{
		// Valid account #1
		{
			types.StringToAddress("123"),
			tests.EthToWei(50), // 50 ETH
		},

		// Valid account #2
		{
			types.StringToAddress("456"),
			tests.EthToWei(10), // 10 ETH
		},

		// Valid account #3
		{
			types.StringToAddress("789"),
			tests.EthToWei(0), // 0 ETH
		},
	}

	hub := GetStakingHub()

	// Instantiate the staking map
	var mappings []testStakeMapping
	for _, stakePair := range validStakes {
		mappings = append(mappings, testStakeMapping{
			address: stakePair.address,
			stake:   stakePair.initialStake,
		})
	}
	instantiateStakingMap(mappings)

	// Reset the stake
	for _, stakePair := range validStakes {
		hub.ResetStake(stakePair.address)

		assert.Equalf(t, big.NewInt(0), hub.GetStakedBalance(stakePair.address), "Stake amount not reset")
	}
}
