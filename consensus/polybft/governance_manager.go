package polybft

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
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

// GovernanceManager interface provides functions for handling governance events
// and updating client configuration based on executed governance proposals
type GovernanceManager interface {
	PostBlock(req *PostBlockRequest) error
	PostEpoch(req *PostEpochRequest) error
	GetClientConfig() (*PolyBFTConfig, error)
}

var _ GovernanceManager = (*dummyGovernanceManager)(nil)

// dummyStakeManager is a dummy implementation of GovernanceManager interface
// used only for unit testing
type dummyGovernanceManager struct{}

func (d *dummyGovernanceManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyGovernanceManager) PostEpoch(req *PostEpochRequest) error { return nil }
func (d *dummyGovernanceManager) GetClientConfig() (*PolyBFTConfig, error) {
	return nil, nil
}

var _ GovernanceManager = (*governanceManager)(nil)

// governanceManager is a struct that saves governance events
// and updates the client configuration based on executed governance proposals
type governanceManager struct {
	logger         hclog.Logger
	state          *State
	eventsGetter   *eventsGetter[contractsapi.EventAbi]
	allForksHashes map[types.Hash]string
}

// newGovernanceManager is a constructor function for governance manager
func newGovernanceManager(genesisConfig *PolyBFTConfig,
	logger hclog.Logger,
	state *State,
	blockhain blockchainBackend) (*governanceManager, error) {
	eventsGetter := &eventsGetter[contractsapi.EventAbi]{
		blockchain: blockhain,
		isValidLogFn: func(l *types.Log) bool {
			return l.Address == genesisConfig.GovernanceConfig.NetworkParamsAddr ||
				l.Address == genesisConfig.GovernanceConfig.ForkParamsAddr
		},
		parseEventFn: parseGovernanceEvent,
	}

	config, err := state.GovernanceStore.getClientConfig()
	if config == nil || errors.Is(err, errClientConfigNotFound) {
		// insert initial config to db if not already inserted
		if err = state.GovernanceStore.insertClientConfig(genesisConfig); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// cache all fork name hashes that we have in code
	allForkNameHashes := map[types.Hash]string{}

	for name := range *chain.AllForksEnabled {
		encoded, err := abi.Encode([]interface{}{name}, abi.MustNewType("tuple(string)"))
		if err != nil {
			return nil, fmt.Errorf("could not encode fork name: %s. Error: %w", name, err)
		}

		forkHash := crypto.Keccak256Hash(encoded)
		allForkNameHashes[forkHash] = name
	}

	g := &governanceManager{
		logger:         logger,
		state:          state,
		eventsGetter:   eventsGetter,
		allForksHashes: allForkNameHashes,
	}

	// get all forks we already have in db and activate them on startup
	forksInDB, err := state.GovernanceStore.getAllForkEvents()
	if err != nil {
		return nil, fmt.Errorf("could not activate forks from db on startup. Error: %w", err)
	}

	lastBuiltBlock := blockhain.CurrentHeader().Number

	for fork, block := range forksInDB {
		if err := g.activateSingleFork(lastBuiltBlock, fork, block); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// GetClientConfig returns latest client configuration from boltdb
func (g *governanceManager) GetClientConfig() (*PolyBFTConfig, error) {
	return g.state.GovernanceStore.getClientConfig()
}

// PostEpoch notifies the governance manager that an epoch has changed
func (g *governanceManager) PostEpoch(req *PostEpochRequest) error {
	if !forkmanager.GetInstance().IsForkEnabled(chain.Governance, req.FirstBlockOfEpoch) {
		// if governance fork is not enabled, do nothing
		return nil
	}

	previousEpoch := req.NewEpochID - 1
	lastInsertedBlock := req.FirstBlockOfEpoch - 1

	g.logger.Debug("Post epoch - getting events from executed governance proposals...",
		"epoch", previousEpoch)

	// if last PostBlock failed to save events, we need to try to get them again
	// to have correct configuration at beginning of next epoch
	lastProcessed, err := g.state.GovernanceStore.getLastProcessed()
	if err != nil {
		return err
	}

	missedEvents, err := g.eventsGetter.getFromBlocksWithToBlock(lastProcessed, lastInsertedBlock)
	if err != nil {
		return err
	}

	if err := g.state.GovernanceStore.insertGovernanceEvents(previousEpoch,
		lastInsertedBlock, missedEvents); err != nil {
		return err
	}

	// if last PostBlock failed to save events, and we gotten the ones we missed,
	// check if there is some event for activation of a new fork, and activate it
	if err := g.activateNewForks(lastInsertedBlock, missedEvents); err != nil {
		g.logger.Error("Post epoch - Could not activate forks based on ForkParams events.", "err", err)

		return err
	}

	// get events that happened in the previous epoch
	eventsRaw, err := g.state.GovernanceStore.getNetworkParamsEvents(previousEpoch)
	if err != nil {
		return fmt.Errorf("could not get governance events on start of epoch: %d. %w",
			req.NewEpochID, err)
	}

	if len(eventsRaw) == 0 {
		// valid situation, no governance proposals were executed in previous epoch
		g.logger.Debug("Post epoch - no executed governance proposals happened in epoch",
			"epoch", previousEpoch)

		return nil
	}

	// get last saved config
	lastConfig, err := g.state.GovernanceStore.getClientConfig()
	if err != nil {
		return err
	}

	var (
		checkpointIntervalEvent  contractsapi.NewCheckpointBlockIntervalEvent
		epochSizeEvent           contractsapi.NewEpochSizeEvent
		epochRewardEvent         contractsapi.NewEpochRewardEvent
		minValidatorSetSizeEvent contractsapi.NewMinValidatorSetSizeEvent
		maxValidatorSetSizeEvent contractsapi.NewMaxValidatorSetSizeEvent
		withdrawalPeriodEvent    contractsapi.NewWithdrawalWaitPeriodEvent
		blockTimeEvent           contractsapi.NewBlockTimeEvent
		blockTimeDriftEvent      contractsapi.NewBlockTimeDriftEvent
		votingDelayEvent         contractsapi.NewVotingDelayEvent
		votingPeriodEvent        contractsapi.NewVotingPeriodEvent
		proposalThresholdEvent   contractsapi.NewProposalThresholdEvent
		sprintSizeEvent          contractsapi.NewSprintSizeEvent
	)

	// unmarshal events that happened in previous epoch and update last saved config
	for _, e := range eventsRaw {
		switch ethgo.Hash(e[:types.HashLength]) {
		case checkpointIntervalEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewCheckpointBlockIntervalEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewCheckpointBlockIntervalEvent: %w", err)
			}

			lastConfig.CheckpointInterval = event.CheckpointInterval.Uint64()
			g.logger.Debug("Post epoch - Checkpoint block interval changed in governance",
				"epoch", previousEpoch, "checkpointBlockInterval", lastConfig.CheckpointInterval)

		case epochSizeEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewEpochSizeEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewEpochSizeEvent: %w", err)
			}

			lastConfig.EpochSize = event.Size.Uint64()
			g.logger.Debug("Post epoch - Epoch size changed in governance",
				"epoch", previousEpoch, "epochSize", lastConfig.EpochSize)

		case epochRewardEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewEpochRewardEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewEpochRewardEvent: %w", err)
			}

			lastConfig.EpochReward = event.Reward.Uint64()
			g.logger.Debug("Post epoch - Epoch reward changed in governance",
				"epoch", previousEpoch, "epochReward", lastConfig.EpochReward)

		case minValidatorSetSizeEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewMinValidatorSetSizeEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewMinValidatorSetSizeEvent: %w", err)
			}

			lastConfig.MinValidatorSetSize = event.MinValidatorSet.Uint64()
			g.logger.Debug("Post epoch - Min validator set size changed in governance",
				"epoch", previousEpoch, "minValidatorSetSize", lastConfig.MinValidatorSetSize)

		case maxValidatorSetSizeEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewMaxValidatorSetSizeEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewMaxValdidatorSetSizeEvent: %w", err)
			}

			lastConfig.MaxValidatorSetSize = event.MaxValidatorSet.Uint64()
			g.logger.Debug("Post epoch - Max validator set size changed in governance",
				"epoch", previousEpoch, "maxValidatorSetSize", lastConfig.MaxValidatorSetSize)

		case withdrawalPeriodEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewWithdrawalWaitPeriodEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewWithdrawalWaitPeriodEvent: %w", err)
			}

			lastConfig.WithdrawalWaitPeriod = event.WithdrawalPeriod.Uint64()
			g.logger.Debug("Post epoch - Withdrawal wait period changed in governance",
				"epoch", previousEpoch, "withdrawalWaitPeriod", lastConfig.WithdrawalWaitPeriod)

		case blockTimeEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewBlockTimeEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewBlockTimeEvent: %w", err)
			}

			lastConfig.BlockTime = common.Duration{Duration: time.Duration(event.BlockTime.Int64()) * time.Second}
			g.logger.Debug("Post epoch - Block time changed in governance",
				"epoch", previousEpoch, "blockTime", lastConfig.BlockTime)

		case blockTimeDriftEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewBlockTimeDriftEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewBlockTimeDriftEvent: %w", err)
			}

			lastConfig.BlockTimeDrift = event.BlockTimeDrift.Uint64()
			g.logger.Debug("Post epoch - Block time drift changed in governance",
				"epoch", previousEpoch, "blockTimeDrift", lastConfig.BlockTimeDrift)

		case votingDelayEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewVotingDelayEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewVotingDelayEvent: %w", err)
			}

			lastConfig.GovernanceConfig.VotingDelay = event.VotingDelay
			g.logger.Debug("Post epoch - Voting delay changed in governance",
				"epoch", previousEpoch, "votingDelay", lastConfig.GovernanceConfig.VotingDelay)

		case votingPeriodEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewVotingPeriodEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewVotingPeriodEvent: %w", err)
			}

			lastConfig.GovernanceConfig.VotingPeriod = event.VotingPeriod
			g.logger.Debug("Post epoch - Voting period changed in governance",
				"epoch", previousEpoch, "votingPeriod", lastConfig.GovernanceConfig.VotingPeriod)

		case proposalThresholdEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewProposalThresholdEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewProposalThresholdEvent: %w", err)
			}

			lastConfig.GovernanceConfig.ProposalThreshold = event.ProposalThreshold
			g.logger.Debug("Post epoch - Proposal threshold changed in governance",
				"epoch", previousEpoch, "proposalThreshold", lastConfig.GovernanceConfig.ProposalThreshold)

		case sprintSizeEvent.Sig():
			event, err := unmarshalGovernanceEvent[*contractsapi.NewSprintSizeEvent](e)
			if err != nil {
				return fmt.Errorf("could not unmarshal NewSprintSizeEvent: %w", err)
			}

			lastConfig.SprintSize = event.Size.Uint64()
			g.logger.Debug("Post epoch - Sprint size changed in governance",
				"epoch", previousEpoch, "sprintSize", lastConfig.SprintSize)

		default:
			return errUnknownGovernanceEvent
		}
	}

	// save updated config to db
	return g.state.GovernanceStore.insertClientConfig(lastConfig)
}

// PostBlock notifies governance manager that a block was finalized
// so that he can extract governance events and save them to bolt db
func (g *governanceManager) PostBlock(req *PostBlockRequest) error {
	if !forkmanager.GetInstance().IsForkEnabled(chain.Governance, req.FullBlock.Block.Number()) {
		// if governance fork is not enabled, do nothing
		return nil
	}

	lastBlock, err := g.state.GovernanceStore.getLastProcessed()
	if err != nil {
		return fmt.Errorf("could not get last processed block for governance events. Error: %w", err)
	}

	g.logger.Debug("Post block - Getting governance events...", "epoch", req.Epoch,
		"block", req.FullBlock.Block.Number(), "lastGottenBlock", lastBlock)

	governanceEvents, err := g.eventsGetter.getFromBlocks(lastBlock, req.FullBlock)
	if err != nil {
		return err
	}

	currentBlock := req.FullBlock.Block.Number()

	numOfEvents := len(governanceEvents)
	if numOfEvents == 0 {
		g.logger.Debug("Post block - Getting governance events finished, no events found.",
			"epoch", req.Epoch, "block", currentBlock, "lastGottenBlock", lastBlock)

		// even if there were no governance events in block, mark it as last processed
		return g.state.GovernanceStore.insertLastProcessed(currentBlock)
	}

	g.logger.Debug("Post block - Getting governance events done.", "epoch", req.Epoch,
		"block", currentBlock, "eventsNum", numOfEvents)

	if err := g.activateNewForks(currentBlock, governanceEvents); err != nil {
		g.logger.Error("Post block - Could not activate forks based on ForkParams events.", "err", err)

		return err
	}

	// this will also update the last processed block
	return g.state.GovernanceStore.insertGovernanceEvents(
		req.Epoch,
		req.FullBlock.Block.Number(),
		governanceEvents)
}

// activateNewForks activates forks based on the ForkParams contract events
func (g *governanceManager) activateNewForks(currentBlock uint64,
	governanceEvents []contractsapi.EventAbi) error {
	for _, e := range governanceEvents {
		if forkHash, block, isForkEvent := isForkParamsEvent(e); isForkEvent {
			if err := g.activateSingleFork(currentBlock, forkHash, block); err != nil {
				return err
			}
		}
	}

	return nil
}

// activateSingleFork registers (if not already registered) and activates fork from specified block
// if given fork does not exist in code (the code version is not up to data to that fork), it will panic
func (g *governanceManager) activateSingleFork(currentBlock uint64, forkHash types.Hash, block *big.Int) error {
	forkManager := forkmanager.GetInstance()

	// here we add registration and activation of forks
	// based on ForkParams contract
	forkName, exists := g.allForksHashes[forkHash]
	if !exists {
		// current code version does not have this fork
		// so we stop the node
		panic(fmt.Sprintf("current version of code does not have this fork: %v", forkHash)) //nolint:gocritic
	}

	g.logger.Debug("Gotten event for fork activation", "forkName", forkName,
		"forkHash", forkHash, "block", block.Uint64())

	if !forkManager.IsForkRegistered(forkName) {
		// if fork is not already registered, register it
		forkManager.RegisterFork(forkName, nil)
	}

	if forkManager.IsForkEnabled(forkName, currentBlock) {
		// this fork is already enabled and up and running,
		// if an event happens to activate it from some later block
		// we will first deactivate it, and activate it again from the new specified block
		if err := forkManager.DeactivateFork(forkName); err != nil {
			return fmt.Errorf("could not deactivate fork: %s. Error: %w", forkName, err)
		}
	}

	blockNum := block.Uint64()
	if err := forkManager.ActivateFork(forkName, blockNum); err != nil {
		// activate fork for given number
		return fmt.Errorf("could not activate fork: %s for block: %d based on ForkParams event",
			forkName, blockNum)
	}

	g.logger.Debug("Registered and activated fork", "forkName", forkName,
		"forkHash", forkHash, "block", block.Uint64())

	return nil
}

// isForkParamsEvent returns true if given contractsapi event is an event
// from ForkParams contract, and returns its feature hash and blockNumber as well
func isForkParamsEvent(event contractsapi.EventAbi) (types.Hash, *big.Int, bool) {
	switch obj := event.(type) {
	case *contractsapi.NewFeatureEvent:
		return obj.Feature, obj.Block, true
	case *contractsapi.UpdatedFeatureEvent:
		return obj.Feature, obj.Block, true
	default:
		return types.ZeroHash, nil, false
	}
}

// parseGovernanceEvent parses provided log to correct governance event
func parseGovernanceEvent(h *types.Header, log *ethgo.Log) (contractsapi.EventAbi, bool, error) {
	var (
		checkpointIntervalEvent  contractsapi.NewCheckpointBlockIntervalEvent
		epochSizeEvent           contractsapi.NewEpochSizeEvent
		epochRewardEvent         contractsapi.NewEpochRewardEvent
		minValidatorSetSizeEvent contractsapi.NewMinValidatorSetSizeEvent
		maxValidatorSetSizeEvent contractsapi.NewMaxValidatorSetSizeEvent
		withdrawalPeriodEvent    contractsapi.NewWithdrawalWaitPeriodEvent
		blockTimeEvent           contractsapi.NewBlockTimeEvent
		blockTimeDriftEvent      contractsapi.NewBlockTimeDriftEvent
		votingDelayEvent         contractsapi.NewVotingDelayEvent
		votingPeriodEvent        contractsapi.NewVotingPeriodEvent
		proposalThresholdEvent   contractsapi.NewProposalThresholdEvent
		sprintSizeEvent          contractsapi.NewSprintSizeEvent
		newFeatureEvent          contractsapi.NewFeatureEvent
		updatedFeatureEvent      contractsapi.UpdatedFeatureEvent
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
	case sprintSizeEvent.Sig():
		return parseEvent(&sprintSizeEvent)
	case newFeatureEvent.Sig():
		return parseEvent(&newFeatureEvent)
	case updatedFeatureEvent.Sig():
		return parseEvent(&updatedFeatureEvent)
	default:
		return nil, false, errUnknownGovernanceEvent
	}
}

// unmarshalGovernanceEvent unmarshals given raw event to desired type
func unmarshalGovernanceEvent[T contractsapi.EventAbi](rawEvent []byte) (T, error) {
	var event T

	err := json.Unmarshal(rawEvent[types.HashLength:], &event)

	return event, err
}
