package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
	"github.com/umbracle/ethgo/abi"
)

func TestGovernanceManager_PostEpoch(t *testing.T) {
	t.Parallel()

	state := newTestState(t)
	governanceManager := &governanceManager{
		state:  state,
		logger: hclog.NewNullLogger(),
	}

	// insert some governance event
	baseFeeChangeDenomEvent := &contractsapi.NewBaseFeeChangeDenomEvent{BaseFeeChangeDenom: big.NewInt(100)}
	epochRewardEvent := &contractsapi.NewEpochRewardEvent{Reward: big.NewInt(10000)}

	require.NoError(t, state.GovernanceStore.insertGovernanceEvent(1, baseFeeChangeDenomEvent, nil))
	require.NoError(t, state.GovernanceStore.insertGovernanceEvent(1, epochRewardEvent, nil))

	// no initial config was saved, so we expect an error
	require.ErrorIs(t, governanceManager.PostEpoch(&PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
		Forks:             &chain.Forks{chain.Governance: chain.NewFork(0)},
	}),
		errClientConfigNotFound)

	params := &chain.Params{
		BaseFeeChangeDenom: 8,
		Engine:             map[string]interface{}{ConsensusName: createTestPolybftConfig()},
	}

	// insert initial config
	require.NoError(t, state.GovernanceStore.insertClientConfig(params, nil))

	// PostEpoch will now update config with new epoch reward value
	require.NoError(t, governanceManager.PostEpoch(&PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
		Forks:             &chain.Forks{chain.Governance: chain.NewFork(0)},
	}))

	updatedConfig, err := state.GovernanceStore.getClientConfig(nil)
	require.NoError(t, err)
	require.Equal(t, baseFeeChangeDenomEvent.BaseFeeChangeDenom.Uint64(), updatedConfig.BaseFeeChangeDenom)

	pbftConfig, err := GetPolyBFTConfig(updatedConfig)
	require.NoError(t, err)

	require.Equal(t, epochRewardEvent.Reward.Uint64(), pbftConfig.EpochReward)
}

func TestGovernanceManager_PostBlock(t *testing.T) {
	t.Parallel()

	genesisPolybftConfig := createTestPolybftConfig()

	t.Run("Has no events in block", func(t *testing.T) {
		t.Parallel()

		state := newTestState(t)

		// no governance events in receipts
		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: 5}},
				Receipts: []*types.Receipt{},
			},
			Epoch: 1,
			Forks: &chain.Forks{chain.Governance: chain.NewFork(0)},
		}

		blockchainMock := new(blockchainMock)
		blockchainMock.On("CurrentHeader").Return(&types.Header{
			Number: 0,
		})

		chainParams := &chain.Params{Engine: map[string]interface{}{ConsensusName: genesisPolybftConfig}}
		governanceManager, err := newGovernanceManager(chainParams,
			hclog.NewNullLogger(), state, blockchainMock, nil)
		require.NoError(t, err)

		require.NoError(t, governanceManager.PostBlock(req))

		eventsRaw, err := state.GovernanceStore.getNetworkParamsEvents(1, nil)
		require.NoError(t, err)
		require.Len(t, eventsRaw, 0)
	})

	t.Run("Has new fork", func(t *testing.T) {
		t.Parallel()

		var (
			newForkHash  = types.StringToHash("0xNewForkHash")
			newForkBlock = big.NewInt(5)
			newForkName  = "newFork"
		)

		state := newTestState(t)

		req := &PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: 5}}},
			Epoch:     1,
			Forks:     &chain.Forks{chain.Governance: chain.NewFork(0)},
		}

		blockchainMock := new(blockchainMock)
		blockchainMock.On("CurrentHeader").Return(&types.Header{
			Number: 4,
		})

		chainParams := &chain.Params{Engine: map[string]interface{}{ConsensusName: genesisPolybftConfig}}
		governanceManager, err := newGovernanceManager(chainParams,
			hclog.NewNullLogger(), state, blockchainMock, nil)
		require.NoError(t, err)

		// this cheats that we have this fork in code
		governanceManager.allForksHashes[newForkHash] = newForkName

		require.NoError(t, state.GovernanceStore.insertGovernanceEvent(1,
			&contractsapi.NewFeatureEvent{
				Feature: newForkHash, Block: newForkBlock,
			}, nil))

		// new fork should not be registered and enabled before PostBlock
		require.False(t, forkmanager.GetInstance().IsForkEnabled(newForkName, newForkBlock.Uint64()))

		require.NoError(t, governanceManager.PostBlock(req))

		// new fork should be registered and enabled before PostBlock
		require.True(t, forkmanager.GetInstance().IsForkEnabled(newForkName, newForkBlock.Uint64()))
	})
}

func createTestLogForNewEpochSizeEvent(t *testing.T, epochSize uint64) *types.Log {
	t.Helper()

	var epochSizeEvent contractsapi.NewEpochSizeEvent

	topics := make([]types.Hash, 2)
	topics[0] = types.Hash(epochSizeEvent.Sig())
	encodedData, err := abi.MustNewType("uint256").Encode(new(big.Int).SetUint64(epochSize))
	require.NoError(t, err)

	topics[1] = types.BytesToHash(encodedData)

	return &types.Log{
		Address: contracts.NetworkParamsContract,
		Topics:  topics,
		Data:    nil,
	}
}
