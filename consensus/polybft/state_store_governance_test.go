package polybft

import (
	"math/big"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestGovernanceStore_InsertAndGetEvents(t *testing.T) {
	t.Parallel()

	epoch := uint64(11)
	block := uint64(111)
	state := newTestState(t)

	checkpointIntervalEvent := &contractsapi.NewCheckpointBlockIntervalEvent{CheckpointInterval: big.NewInt(900)}
	epochSizeEvent := &contractsapi.NewEpochSizeEvent{Size: big.NewInt(10)}
	epochRewardEvent := &contractsapi.NewEpochRewardEvent{Reward: big.NewInt(1000)}
	minValidatorSetSizeEvent := &contractsapi.NewMinValidatorSetSizeEvent{MinValidatorSet: big.NewInt(4)}
	maxValidatorSetSizeEvent := &contractsapi.NewMaxValidatorSetSizeEvent{MaxValidatorSet: big.NewInt(100)}
	withdrawalPeriodEvent := &contractsapi.NewWithdrawalWaitPeriodEvent{WithdrawalPeriod: big.NewInt(1)}
	blockTimeEvent := &contractsapi.NewBlockTimeEvent{BlockTime: big.NewInt(2)}
	blockTimeDriftEvent := &contractsapi.NewBlockTimeDriftEvent{BlockTimeDrift: big.NewInt(10)}
	votingDelayEvent := &contractsapi.NewVotingDelayEvent{VotingDelay: big.NewInt(1000)}
	votingPeriodEvent := &contractsapi.NewVotingPeriodEvent{VotingPeriod: big.NewInt(10_000)}
	proposalThresholdEvent := &contractsapi.NewProposalThresholdEvent{ProposalThreshold: big.NewInt(1000)}
	sprintSizeEvent := &contractsapi.NewSprintSizeEvent{Size: big.NewInt(7)}

	events := []contractsapi.EventAbi{
		checkpointIntervalEvent,
		epochSizeEvent,
		epochRewardEvent,
		minValidatorSetSizeEvent,
		maxValidatorSetSizeEvent,
		withdrawalPeriodEvent,
		blockTimeEvent,
		blockTimeDriftEvent,
		votingDelayEvent,
		votingPeriodEvent,
		proposalThresholdEvent,
	}

	require.NoError(t, state.GovernanceStore.insertGovernanceEvents(epoch, block, events))

	// test for an epoch that didn't have any events
	eventsRaw, err := state.GovernanceStore.getGovernanceEvents(10)
	require.NoError(t, err)
	require.Len(t, eventsRaw, 0)

	// test for the epoch that had events
	eventsRaw, err = state.GovernanceStore.getGovernanceEvents(epoch)
	require.NoError(t, err)
	require.Len(t, eventsRaw, len(events))

	lastProcessedBlock, err := state.GovernanceStore.getLastProcessed()
	require.NoError(t, err)
	require.Equal(t, block, lastProcessedBlock)

	// insert some more events for current epoch
	require.NoError(t, state.GovernanceStore.insertGovernanceEvents(epoch, block+1,
		[]contractsapi.EventAbi{sprintSizeEvent}))

	eventsRaw, err = state.GovernanceStore.getGovernanceEvents(epoch)
	require.NoError(t, err)
	require.Len(t, eventsRaw, len(events)+1)

	lastProcessedBlock, err = state.GovernanceStore.getLastProcessed()
	require.NoError(t, err)
	require.Equal(t, block+1, lastProcessedBlock)
}

func TestGovernanceStore_InsertAndGetClientConfig(t *testing.T) {
	t.Parallel()

	initialConfig := createTestPolybftConfig()
	state := newTestState(t)

	// try get config when there is none
	_, err := state.GovernanceStore.getClientConfig()
	require.ErrorIs(t, err, errClientConfigNotFound)

	// insert config
	require.NoError(t, state.GovernanceStore.insertClientConfig(initialConfig))

	// now config should exist
	configFromDB, err := state.GovernanceStore.getClientConfig()
	require.NoError(t, err)
	// check some fields to make sure they are as expected
	require.Len(t, configFromDB.InitialValidatorSet, len(initialConfig.InitialValidatorSet))
	require.Equal(t, configFromDB.BlockTime, initialConfig.BlockTime)
	require.Equal(t, configFromDB.BlockTimeDrift, initialConfig.BlockTimeDrift)
	require.Equal(t, configFromDB.CheckpointInterval, initialConfig.CheckpointInterval)
	require.Equal(t, configFromDB.EpochReward, initialConfig.EpochReward)
	require.Equal(t, configFromDB.EpochSize, initialConfig.EpochSize)
	require.Equal(t, configFromDB.Governance, initialConfig.Governance)
}

func createTestPolybftConfig() *PolyBFTConfig {
	return &PolyBFTConfig{
		InitialValidatorSet: []*validator.GenesisValidator{
			{
				Address: types.BytesToAddress([]byte{0, 1, 2}),
				Stake:   big.NewInt(100),
			},
			{
				Address: types.BytesToAddress([]byte{3, 4, 5}),
				Stake:   big.NewInt(100),
			},
			{
				Address: types.BytesToAddress([]byte{6, 7, 8}),
				Stake:   big.NewInt(100),
			},
			{
				Address: types.BytesToAddress([]byte{9, 10, 11}),
				Stake:   big.NewInt(100),
			},
		},
		Bridge: &BridgeConfig{
			StateSenderAddr:                   types.StringToAddress("0xStateSenderAddr"),
			CheckpointManagerAddr:             types.StringToAddress("0xCheckpointManagerAddr"),
			ExitHelperAddr:                    types.StringToAddress("0xExitHelperAddr"),
			RootERC20PredicateAddr:            types.StringToAddress("0xRootERC20PredicateAddr"),
			ChildMintableERC20PredicateAddr:   types.StringToAddress("0xChildMintableERC20PredicateAddr"),
			RootNativeERC20Addr:               types.StringToAddress("0xRootNativeERC20Addr"),
			RootERC721PredicateAddr:           types.StringToAddress("0xRootERC721PredicateAddr"),
			ChildMintableERC721PredicateAddr:  types.StringToAddress("0xChildMintableERC721PredicateAddr"),
			RootERC1155PredicateAddr:          types.StringToAddress("0xRootERC1155PredicateAddr"),
			ChildMintableERC1155PredicateAddr: types.StringToAddress("0xChildMintableERC1155PredicateAddr"),
			ChildERC20Addr:                    types.StringToAddress("0xChildERC20Addr"),
			ChildERC721Addr:                   types.StringToAddress("0xChildERC721Addr"),
			ChildERC1155Addr:                  types.StringToAddress("0xChildERC1155Addr"),
			CustomSupernetManagerAddr:         types.StringToAddress("0xCustomSupernetManagerAddr"),
			StakeManagerAddr:                  types.StringToAddress("0xStakeManagerAddr"),
			StakeTokenAddr:                    types.StringToAddress("0xStakeTokenAddr"),
			BLSAddress:                        types.StringToAddress("0xBLSAddress"),
			BN256G2Address:                    types.StringToAddress("0xBN256G2Address"),
			JSONRPCEndpoint:                   "http://mumbai-rpc.com",
			EventTrackerStartBlocks: map[types.Address]uint64{
				types.StringToAddress("SomeRootAddress"): 365_000,
			},
		},
		EpochSize:           10,
		EpochReward:         1000,
		SprintSize:          5,
		BlockTime:           common.Duration{Duration: 2 * time.Second},
		MinValidatorSetSize: 4,
		MaxValidatorSetSize: 100,
		SupernetID:          11,
		CheckpointInterval:  900,
		BlockTimeDrift:      10,
		Governance:          types.ZeroAddress,
		NativeTokenConfig: &TokenConfig{
			Name:       "Polygon_MATIC",
			Symbol:     "MATIC",
			Decimals:   18,
			IsMintable: false,
			Owner:      types.ZeroAddress,
		},
		InitialTrieRoot:      types.ZeroHash,
		WithdrawalWaitPeriod: 1,
		RewardConfig: &RewardsConfig{
			TokenAddress:  types.StringToAddress("0xRewardTokenAddr"),
			WalletAddress: types.StringToAddress("0xRewardWalletAddr"),
			WalletAmount:  big.NewInt(1_000_000),
		},
		GovernanceConfig: &GovernanceConfig{
			VotingDelay:              big.NewInt(1000),
			VotingPeriod:             big.NewInt(10_0000),
			ProposalThreshold:        big.NewInt(1000),
			GovernorAdmin:            types.StringToAddress("0xGovernorAdmin"),
			ProposalQuorumPercentage: 67,
		},
	}
}
