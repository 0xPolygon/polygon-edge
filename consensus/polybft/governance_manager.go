package polybft

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

const (
	oldRewardLookbackSize = uint64(2) // number of blocks to calculate commit epoch info from the previous epoch
	newRewardLookbackSize = uint64(1)
)

var errUnknownGovernanceEvent = errors.New("unknown event from governance")

// isRewardDistributionBlock indicates if reward distribution transaction
// should happen in given block
// if governance fork is enabled, reward distribution is only present on the first block of epoch
// and if we are not at the start of chain
// if governance fork is not enabled, reward distribution is only present at the epoch ending block
func isRewardDistributionBlock(isFirstBlockOfEpoch, isEndOfEpoch bool,
	pendingBlockNumber uint64) bool {
	if forkmanager.GetInstance().IsForkEnabled(chain.Governance, pendingBlockNumber) {
		return isFirstBlockOfEpoch && pendingBlockNumber > 1
	}

	return isEndOfEpoch
}

// getLookbackSizeForRewardDistribution returns lookback size for reward distribution
// based on if governance fork is enabled or not
func getLookbackSizeForRewardDistribution(blockNumber uint64) uint64 {
	if forkmanager.GetInstance().IsForkEnabled(chain.Governance, blockNumber) {
		return newRewardLookbackSize
	}

	return oldRewardLookbackSize
}

// governanceManager is a struct that saves governance events
// and updates the client configuration based on executed governance proposals
type governanceManager struct {
	logger       hclog.Logger
	state        *State
	eventsGetter *eventsGetter[contractsapi.EventAbi]
}

// newGovernanceManager is a constructor function for governance manager
func newGovernanceManager(logger hclog.Logger, state *State, blockhain blockchainBackend) *governanceManager {
	eventsGetter := &eventsGetter[contractsapi.EventAbi]{
		blockchain: blockhain,
		isValidLogFn: func(l *types.Log) bool {
			return l.Address == contracts.NetworkParamsContract
		},
		parseEventFn: parseGovernanceEvent,
	}

	return &governanceManager{
		logger:       logger,
		state:        state,
		eventsGetter: eventsGetter,
	}
}

// PostEpoch notifies the governance manager that an epoch has changed
func (g *governanceManager) PostEpoch(req *PostEpochRequest) error {
	if !forkmanager.GetInstance().IsForkEnabled(chain.Governance, req.FirstBlockOfEpoch) {
		// if governance fork is not enabled, do nothing
		return nil
	}

	// get events that happened in the previous epoch
	eventsRaw, err := g.state.GovernanceStore.getGovernanceEvents(req.NewEpochID - 1)
	if err != nil {
		return fmt.Errorf("could not get governance events on start of epoch: %d. %w",
			req.NewEpochID, err)
	}

	if len(eventsRaw) == 0 {
		// valid situation, no governance proposals were executed in previous epoch
		return nil
	}

	// TODO - update configuration

	return nil
}

// PostBlock notifies governance manager that a block was finalized
// so that he can extract governance events and save them to bolt db
func (g *governanceManager) PostBlock(req *PostBlockRequest) error {
	if !forkmanager.GetInstance().IsForkEnabled(chain.Governance, req.FullBlock.Block.Number()) {
		// if governance fork is not enabled, do nothing
		return nil
	}

	lastBlock, err := g.state.GovernanceStore.getLastSaved()
	if err != nil {
		return fmt.Errorf("could not get last processed block for governance events. Error: %w", err)
	}

	governanceEvents, err := g.eventsGetter.getFromBlocks(lastBlock, req.FullBlock)
	if err != nil {
		return err
	}

	return g.state.GovernanceStore.insertGovernanceEvents(
		req.Epoch,
		req.FullBlock.Block.Number(),
		governanceEvents)
}

// parseGovernanceEvent parses provided log to correct governance event
func parseGovernanceEvent(h *types.Header, log *ethgo.Log) (contractsapi.EventAbi, bool, error) {
	var (
		checkpointIntervalEvent  contractsapi.NewCheckpointBlockIntervalEvent
		epochSizeEvent           contractsapi.NewEpochSizeEvent
		epochRewardEvent         contractsapi.NewEpochRewardEvent
		minValidatorSetSizeEvent contractsapi.NewMinValidatorSetSizeEvent
		maxValidatorSetSizeEvent contractsapi.NewMaxValdidatorSetSizeEvent
		withdrawalPeriodEvent    contractsapi.NewWithdrawalWaitPeriodEvent
		blockTimeEvent           contractsapi.NewBlockTimeEvent
		blockTimeDriftEvent      contractsapi.NewBlockTimeDriftEvent
		votingDelayEvent         contractsapi.NewVotingDelayEvent
		votingPeriodEvent        contractsapi.NewVotingPeriodEvent
		proposalThresholdEvent   contractsapi.NewProposalThresholdEvent
	)

	parseEvent := func(event contractsapi.EventAbi) (contractsapi.EventAbi, bool, error) {
		doesMatch, err := event.ParseLog(log)

		return event, doesMatch, err
	}

	switch log.Topics[0] {
	case checkpointIntervalEvent.Sig():
		return parseEvent(&checkpointIntervalEvent)
	case epochSizeEvent.Sig():
		return parseEvent(&epochSizeEvent)
	case epochRewardEvent.Sig():
		return parseEvent(&epochRewardEvent)
	case minValidatorSetSizeEvent.Sig():
		return parseEvent(&minValidatorSetSizeEvent)
	case maxValidatorSetSizeEvent.Sig():
		return parseEvent(&maxValidatorSetSizeEvent)
	case withdrawalPeriodEvent.Sig():
		return parseEvent(&withdrawalPeriodEvent)
	case blockTimeEvent.Sig():
		return parseEvent(&blockTimeEvent)
	case blockTimeDriftEvent.Sig():
		return parseEvent(&blockTimeDriftEvent)
	case votingDelayEvent.Sig():
		return parseEvent(&votingDelayEvent)
	case votingPeriodEvent.Sig():
		return parseEvent(&votingPeriodEvent)
	case proposalThresholdEvent.Sig():
		return parseEvent(&proposalThresholdEvent)
	default:
		return nil, false, errUnknownGovernanceEvent
	}
}
