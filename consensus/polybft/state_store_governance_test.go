package polybft

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
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
	maxValidatorSetSizeEvent := &contractsapi.NewMaxValdidatorSetSizeEvent{MaxValidatorSet: big.NewInt(100)}
	withdrawalPeriodEvent := &contractsapi.NewWithdrawalWaitPeriodEvent{WithdrawalPeriod: big.NewInt(1)}
	blockTimeEvent := &contractsapi.NewBlockTimeEvent{BlockTime: big.NewInt(2)}
	blockTimeDriftEvent := &contractsapi.NewBlockTimeDriftEvent{BlockTimeDrift: big.NewInt(10)}
	votingDelayEvent := &contractsapi.NewVotingDelayEvent{VotingDelay: big.NewInt(1000)}
	votingPeriodEvent := &contractsapi.NewVotingPeriodEvent{VotingPeriod: big.NewInt(10_000)}
	proposalThresholdEvent := &contractsapi.NewProposalThresholdEvent{ProposalThreshold: big.NewInt(1000)}

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

	// test for the epoch that had events
	eventsRaw, err = state.GovernanceStore.getGovernanceEvents(epoch)
	require.NoError(t, err)
	require.Len(t, eventsRaw, len(events))

	lastProcessedBlock, err := state.GovernanceStore.getLastSaved()
	require.NoError(t, err)
	require.Equal(t, block, lastProcessedBlock)
}
