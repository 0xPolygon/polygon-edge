package polybft

import (
	"encoding/json"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/common"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
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
	epochRewardEvent := &contractsapi.NewEpochRewardEvent{Reward: big.NewInt(10_000)}
	require.NoError(t, state.GovernanceStore.insertGovernanceEvents(1, 7, []contractsapi.EventAbi{epochRewardEvent}))
	// insert last processed block
	require.NoError(t, state.GovernanceStore.insertLastProcessed(20))

	// no initial config was saved, so we expect an error
	require.ErrorIs(t, governanceManager.PostEpoch(&common.PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
		Forks:             &chain.Forks{chain.Governance: chain.NewFork(0)},
	}),
		errClientConfigNotFound)

	// insert initial config
	require.NoError(t, state.GovernanceStore.insertClientConfig(createTestPolybftConfig()))

	// PostEpoch will now update config with new epoch reward value
	require.NoError(t, governanceManager.PostEpoch(&common.PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
		Forks:             &chain.Forks{chain.Governance: chain.NewFork(0)},
	}))

	updatedConfig, err := state.GovernanceStore.getClientConfig()
	require.NoError(t, err)
	require.Equal(t, epochRewardEvent.Reward.Uint64(), updatedConfig.EpochReward)
}

func TestGovernanceManager_PostBlock(t *testing.T) {
	t.Parallel()

	genesisPolybftConfig := createTestPolybftConfig()

	t.Run("Has no events in block", func(t *testing.T) {
		t.Parallel()

		state := newTestState(t)
		require.NoError(t, state.GovernanceStore.insertLastProcessed(4))

		// no governance events in receipts
		req := &common.PostBlockRequest{
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

		governanceManager, err := newGovernanceManager(genesisPolybftConfig,
			hclog.NewNullLogger(), state, blockchainMock)
		require.NoError(t, err)

		require.NoError(t, governanceManager.PostBlock(req))

		eventsRaw, err := state.GovernanceStore.getNetworkParamsEvents(1)
		require.NoError(t, err)
		require.Len(t, eventsRaw, 0)

		// event if there were no governance events, we will mark this block as last processed
		lastBlockProcessed, err := state.GovernanceStore.getLastProcessed()
		require.NoError(t, err)
		require.Equal(t, req.FullBlock.Block.Number(), lastBlockProcessed)
	})

	t.Run("No skipped blocks, had events", func(t *testing.T) {
		t.Parallel()

		newEpochSize := uint64(111_111)

		state := newTestState(t)
		require.NoError(t, state.GovernanceStore.insertLastProcessed(4))

		// has one governance event in receipts
		receipt := &types.Receipt{
			Logs: []*types.Log{
				createTestLogForNewEpochSizeEvent(t, newEpochSize),
			},
		}
		receipt.SetStatus(types.ReceiptSuccess)

		req := &common.PostBlockRequest{
			FullBlock: &types.FullBlock{Block: &types.Block{Header: &types.Header{Number: 5}},
				Receipts: []*types.Receipt{receipt},
			},
			Epoch: 1,
			Forks: &chain.Forks{chain.Governance: chain.NewFork(0)},
		}

		blockchainMock := new(blockchainMock)
		blockchainMock.On("CurrentHeader").Return(&types.Header{
			Number: 4,
		})

		governanceManager, err := newGovernanceManager(genesisPolybftConfig,
			hclog.NewNullLogger(), state, blockchainMock)
		require.NoError(t, err)

		require.NoError(t, governanceManager.PostBlock(req))

		// we should have one governance event in current epoch
		eventsRaw, err := state.GovernanceStore.getNetworkParamsEvents(1)
		require.NoError(t, err)
		require.Len(t, eventsRaw, 1)

		// check if the event is correct
		var epochSizeEvent *contractsapi.NewEpochSizeEvent
		require.NoError(t, json.Unmarshal(eventsRaw[0][32:], &epochSizeEvent))
		require.Equal(t, newEpochSize, epochSizeEvent.Size.Uint64())

		// check if block was marked as last processed
		lastBlockProcessed, err := state.GovernanceStore.getLastProcessed()
		require.NoError(t, err)
		require.Equal(t, req.FullBlock.Block.Number(), lastBlockProcessed)
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
