package polybft

import (
	"math/big"
	"testing"
	"time"

	polyCommon "github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

func TestGovernanceStore_InsertAndGetEvents(t *testing.T) {
	t.Parallel()

	epoch := uint64(11)
	block := uint64(111)
	state := newTestState(t)

	// NetworkParams events
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
	// ForkParams events
	newFeatureEvent := &contractsapi.NewFeatureEvent{Feature: types.BytesToHash([]byte("OxSomeFeature1")),
		Block: big.NewInt(100_000)}
	updateFeatureEvent := &contractsapi.UpdatedFeatureEvent{Feature: types.BytesToHash([]byte("OxSomeFeature2")),
		Block: big.NewInt(150_000)}

	networkParamsEvents := []contractsapi.EventAbi{
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

	forkParamsEvents := []contractsapi.EventAbi{newFeatureEvent, updateFeatureEvent}

	allEvents := make([]contractsapi.EventAbi, 0)
	allEvents = append(allEvents, networkParamsEvents...)
	allEvents = append(allEvents, forkParamsEvents...)

	require.NoError(t, state.GovernanceStore.insertGovernanceEvents(epoch, block, allEvents))

	// test for an epoch that didn't have any events
	eventsRaw, err := state.GovernanceStore.getNetworkParamsEvents(10)
	require.NoError(t, err)
	require.Len(t, eventsRaw, 0)

	// fork events are not saved per epoch so we should have 2
	forksInDB, err := state.GovernanceStore.getAllForkEvents()
	require.NoError(t, err)
	require.Len(t, forksInDB, len(forkParamsEvents))

	// test for the epoch that had events
	eventsRaw, err = state.GovernanceStore.getNetworkParamsEvents(epoch)
	require.NoError(t, err)
	require.Len(t, eventsRaw, len(networkParamsEvents))

	forksInDB, err = state.GovernanceStore.getAllForkEvents()
	require.NoError(t, err)
	require.Len(t, forksInDB, len(forkParamsEvents))

	lastProcessedBlock, err := state.GovernanceStore.getLastProcessed()
	require.NoError(t, err)
	require.Equal(t, block, lastProcessedBlock)

	// insert some more events for current epoch
	newFeatureEventTwo := &contractsapi.UpdatedFeatureEvent{Feature: types.BytesToHash([]byte("OxSomeFeature3")),
		Block: big.NewInt(130_000)}
	require.NoError(t, state.GovernanceStore.insertGovernanceEvents(epoch, block+1,
		[]contractsapi.EventAbi{sprintSizeEvent, newFeatureEventTwo}))

	eventsRaw, err = state.GovernanceStore.getNetworkParamsEvents(epoch)
	require.NoError(t, err)
	require.Len(t, eventsRaw, len(networkParamsEvents)+1)

	forksInDB, err = state.GovernanceStore.getAllForkEvents()
	require.NoError(t, err)
	require.Len(t, forksInDB, len(forkParamsEvents)+1)

	lastProcessedBlock, err = state.GovernanceStore.getLastProcessed()
	require.NoError(t, err)
	require.Equal(t, block+1, lastProcessedBlock)
}

// TODO: FIX
// func TestGovernanceStore_InsertAndGetClientConfig(t *testing.T) {
// 	t.Parallel()

// 	initialConfig := createTestPolybftConfig()
// 	state := newTestState(t)

// 	// try get config when there is none
// 	_, err := state.GovernanceStore.getClientConfig()
// 	require.ErrorIs(t, err, errClientConfigNotFound)

// 	// insert config
// 	require.NoError(t, state.GovernanceStore.insertClientConfig(initialConfig))

// 	// now config should exist
// 	configFromDB, err := state.GovernanceStore.getClientConfig()
// 	require.NoError(t, err)
// 	// check some fields to make sure they are as expected
// 	require.Len(t, configFromDB.InitialValidatorSet, len(initialConfig.InitialValidatorSet))
// 	require.Equal(t, configFromDB.BlockTime, initialConfig.BlockTime)
// 	require.Equal(t, configFromDB.BlockTimeDrift, initialConfig.BlockTimeDrift)
// 	require.Equal(t, configFromDB.CheckpointInterval, initialConfig.CheckpointInterval)
// 	require.Equal(t, configFromDB.EpochReward, initialConfig.EpochReward)
// 	require.Equal(t, configFromDB.EpochSize, initialConfig.EpochSize)
// 	require.Equal(t, configFromDB.Governance, initialConfig.Governance)
// 	require.Equal(t, configFromDB.BaseFeeChangeDenom, initialConfig.BaseFeeChangeDenom)
// }

func createTestPolybftConfig() *polyCommon.PolyBFTConfig {
	return &polyCommon.PolyBFTConfig{
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
		Bridge: &polyCommon.BridgeConfig{
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
		NativeTokenConfig: &polyCommon.TokenConfig{
			Name:       "Polygon_MATIC",
			Symbol:     "MATIC",
			Decimals:   18,
			IsMintable: false,
			Owner:      types.ZeroAddress,
		},
		InitialTrieRoot:      types.ZeroHash,
		WithdrawalWaitPeriod: 1,
		BaseFeeChangeDenom:   20,
		RewardConfig: &polyCommon.RewardsConfig{
			TokenAddress:  types.StringToAddress("0xRewardTokenAddr"),
			WalletAddress: types.StringToAddress("0xRewardWalletAddr"),
			WalletAmount:  big.NewInt(1_000_000),
		},
		GovernanceConfig: &polyCommon.GovernanceConfig{
			VotingDelay:              big.NewInt(1000),
			VotingPeriod:             big.NewInt(10_0000),
			ProposalThreshold:        big.NewInt(1000),
			GovernorAdmin:            types.StringToAddress("0xGovernorAdmin"),
			ProposalQuorumPercentage: 67,
			ChildGovernorAddr:        contracts.ChildGovernorContract,
			ChildTimelockAddr:        contracts.ChildTimelockContract,
			NetworkParamsAddr:        contracts.NetworkParamsContract,
			ForkParamsAddr:           contracts.ForkParamsContract,
		},
	}
}
