package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/require"
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

	// no initial config was saved, so we expect an error
	require.ErrorIs(t, governanceManager.PostEpoch(&PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
	}),
		errClientConfigNotFound)

	// insert initial config
	require.NoError(t, state.GovernanceStore.insertClientConfig(createTestPolybftConfig()))

	// PostEpoch will now update config with new epoch reward value
	require.NoError(t, governanceManager.PostEpoch(&PostEpochRequest{
		NewEpochID:        2,
		FirstBlockOfEpoch: 21,
	}))

	updatedConfig, err := state.GovernanceStore.getClientConfig()
	require.NoError(t, err)
	require.Equal(t, epochRewardEvent.Reward.Uint64(), updatedConfig.EpochReward)
}
