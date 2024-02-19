package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/forkmanager"
	"github.com/0xPolygon/polygon-edge/types"
	bolt "go.etcd.io/bbolt"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	hcf "github.com/hashicorp/go-hclog"
)

const (
	maxCommitmentSize = 10
	stateFileName     = "consensusState.db"
)

var (
	// errNotAValidator represents "node is not a validator" error message
	errNotAValidator = errors.New("node is not a validator")
	// errQuorumNotReached represents "quorum not reached for commitment message" error message
	errQuorumNotReached = errors.New("quorum not reached for commitment message")
)

// txPoolInterface is an abstraction of transaction pool
type txPoolInterface interface {
	Prepare()
	Length() uint64
	Peek() *types.Transaction
	Pop(*types.Transaction)
	Drop(*types.Transaction)
	Demote(*types.Transaction)
	SetSealing(bool)
	ResetWithHeaders(...*types.Header)
}

// epochMetadata is the static info for epoch currently being processed
type epochMetadata struct {
	// Number is the number of the epoch
	Number uint64

	FirstBlockInEpoch uint64

	// Validators is the set of validators for the epoch
	Validators validator.AccountSet

	// CurrentClientConfig is the current client configuration for current epoch
	// that is updated by governance proposals
	CurrentClientConfig *PolyBFTConfig
}

type guardedDataDTO struct {
	// last built block header at the time of collecting data
	lastBuiltBlock *types.Header

	// epoch metadata at the time of collecting data
	epoch *epochMetadata

	// proposerSnapshot at the time of collecting data
	proposerSnapshot *ProposerSnapshot
}

// runtimeConfig is a struct that holds configuration data for given consensus runtime
type runtimeConfig struct {
	genesisParams   *chain.Params
	GenesisConfig   *PolyBFTConfig
	Forks           *chain.Forks
	DataDir         string
	Key             *wallet.Key
	State           *State
	blockchain      blockchainBackend
	polybftBackend  polybftBackend
	txPool          txPoolInterface
	bridgeTopic     topic
	consensusConfig *consensus.Config
	eventTracker    *consensus.EventTracker
}

// consensusRuntime is a struct that provides consensus runtime features like epoch, state and event management
type consensusRuntime struct {
	// config represents wrapper around required parameters which are received from the outside
	config *runtimeConfig

	// state is reference to the struct which encapsulates bridge events persistence logic
	state *State

	// fsm instance which is created for each `runSequence`
	fsm *fsm

	// lock is a lock to access 'epoch' and `lastBuiltBlock`
	lock sync.RWMutex

	// epoch is the metadata for the current epoch
	epoch *epochMetadata

	// lastBuiltBlock is the header of the last processed block
	lastBuiltBlock *types.Header

	// activeValidatorFlag indicates whether the given node is amongst currently active validator set
	activeValidatorFlag atomic.Bool

	// proposerCalculator is the object which manipulates with ProposerSnapshot
	proposerCalculator *ProposerCalculator

	// manager for handling validator stake change and updating validator set
	stakeManager StakeManager

	eventProvider *EventProvider

	// bridgeManager handles storing, processing and executing bridge events
	bridgeManager BridgeManager

	// governanceManager is used for handling governance events gotten from proposals execution
	// also handles updating client configuration based on governance proposals
	governanceManager GovernanceManager

	// logger instance
	logger hcf.Logger
}

// newConsensusRuntime creates and starts a new consensus runtime instance with event tracking
func newConsensusRuntime(log hcf.Logger, config *runtimeConfig) (*consensusRuntime, error) {
	dbTx, err := config.State.beginDBTransaction(true)
	if err != nil {
		return nil, fmt.Errorf("could not begin dbTx to init consensus runtime: %w", err)
	}

	defer dbTx.Rollback() //nolint:errcheck

	proposerCalculator, err := NewProposerCalculator(config, log.Named("proposer_calculator"), dbTx)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus runtime, error while creating proposer calculator %w", err)
	}

	runtime := &consensusRuntime{
		state:              config.State,
		config:             config,
		lastBuiltBlock:     config.blockchain.CurrentHeader(),
		proposerCalculator: proposerCalculator,
		logger:             log.Named("consensus_runtime"),
		eventProvider:      NewEventProvider(config.blockchain),
	}

	bridgeManager, err := newBridgeManager(runtime, config, runtime.eventProvider, log)
	if err != nil {
		return nil, err
	}

	runtime.bridgeManager = bridgeManager

	if err := runtime.initStakeManager(log, dbTx); err != nil {
		return nil, err
	}

	if err := runtime.initGovernanceManager(log, dbTx); err != nil {
		return nil, err
	}

	// we need to call restart epoch on runtime to initialize epoch state
	runtime.epoch, err = runtime.restartEpoch(runtime.lastBuiltBlock, dbTx)
	if err != nil {
		return nil, fmt.Errorf("consensus runtime creation - restart epoch failed: %w", err)
	}

	if err := dbTx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit db tx to init consensus runtime: %w", err)
	}

	return runtime, nil
}

// close is used to tear down allocated resources
func (c *consensusRuntime) close() {
	c.bridgeManager.Close()
}

// initStakeManager initializes stake manager
func (c *consensusRuntime) initStakeManager(logger hcf.Logger, dbTx *bolt.Tx) error {
	var err error

	c.stakeManager, err = newStakeManager(
		logger.Named("stake-manager"),
		c.state,
		contracts.StakeManagerContract,
		c.config.blockchain,
		c.config.polybftBackend,
		dbTx,
	)

	c.eventProvider.Subscribe(c.stakeManager)

	return err
}

// initGovernanceManager initializes governance manager
func (c *consensusRuntime) initGovernanceManager(logger hcf.Logger, dbTx *bolt.Tx) error {
	governanceManager, err := newGovernanceManager(
		c.config.genesisParams,
		c.config.GenesisConfig,
		logger.Named("governance-manager"),
		c.state,
		c.config.blockchain,
		dbTx,
	)

	if err != nil {
		return err
	}

	c.governanceManager = governanceManager
	c.eventProvider.Subscribe(c.governanceManager)

	return nil
}

// getGuardedData returns last build block, proposer snapshot and current epochMetadata in a thread-safe manner.
func (c *consensusRuntime) getGuardedData() (guardedDataDTO, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	lastBuiltBlock := c.lastBuiltBlock.Copy()
	epoch := new(epochMetadata)
	*epoch = *c.epoch // shallow copy, don't need to make validators copy because AccountSet is immutable
	proposerSnapshot, ok := c.proposerCalculator.GetSnapshot()

	if !ok {
		return guardedDataDTO{}, errors.New("cannot collect shared data, snapshot is empty")
	}

	return guardedDataDTO{
		epoch:            epoch,
		lastBuiltBlock:   lastBuiltBlock,
		proposerSnapshot: proposerSnapshot,
	}, nil
}

func (c *consensusRuntime) IsBridgeEnabled() bool {
	// this is enough to check, because bridge config is not something
	// that can be changed through governance
	return c.config.GenesisConfig.IsBridgeEnabled()
}

// OnBlockInserted is called whenever fsm or syncer inserts new block
func (c *consensusRuntime) OnBlockInserted(fullBlock *types.FullBlock) {
	startTime := time.Now().UTC()

	c.lock.Lock()
	defer c.lock.Unlock()

	if c.lastBuiltBlock != nil && c.lastBuiltBlock.Number >= fullBlock.Block.Number() {
		c.logger.Debug("on block inserted already handled",
			"current", c.lastBuiltBlock.Number, "block", fullBlock.Block.Number())

		return
	}

	if err := updateBlockMetrics(fullBlock.Block, c.lastBuiltBlock); err != nil {
		c.logger.Error("failed to update block metrics", "error", err)
	}

	// after the block has been written we reset the txpool so that the old transactions are removed
	c.config.txPool.ResetWithHeaders(fullBlock.Block.Header)

	var (
		epoch = c.epoch
		err   error
		// calculation of epoch and sprint end does not consider slashing currently

		isEndOfEpoch = c.isFixedSizeOfEpochMet(fullBlock.Block.Header.Number, epoch)
	)

	// begin DB transaction
	dbTx, err := c.state.beginDBTransaction(true)
	if err != nil {
		c.logger.Error("failed to begin db transaction on block finalization",
			"block", fullBlock.Block.Number(), "err", err)

		return
	}

	defer dbTx.Rollback() //nolint:errcheck

	lastProcessedEventsBlock, err := c.state.getLastProcessedEventsBlock(dbTx)
	if err != nil {
		c.logger.Error("failed to get last processed events block on block finalization",
			"block", fullBlock.Block.Number(), "err", err)

		return
	}

	if err := c.eventProvider.GetEventsFromBlocks(lastProcessedEventsBlock, fullBlock, dbTx); err != nil {
		c.logger.Error("failed to process events on block finalization", "block", fullBlock.Block.Number(), "err", err)

		return
	}

	postBlock := &PostBlockRequest{
		FullBlock:           fullBlock,
		Epoch:               epoch.Number,
		IsEpochEndingBlock:  isEndOfEpoch,
		DBTx:                dbTx,
		CurrentClientConfig: epoch.CurrentClientConfig,
		Forks:               c.config.Forks,
	}

	// update proposer priorities
	if err := c.proposerCalculator.PostBlock(postBlock); err != nil {
		c.logger.Error("Could not update proposer calculator", "err", err)

		return
	}

	// handle transfer events that happened in block
	if err := c.stakeManager.PostBlock(postBlock); err != nil {
		c.logger.Error("failed to post block in stake manager", "err", err)

		return
	}

	// handle bridge events
	if err := c.bridgeManager.PostBlock(postBlock); err != nil {
		c.logger.Error("failed to post block in bridge manager", "err", err)

		return
	}

	// handle governance events that happened in block
	if err := c.governanceManager.PostBlock(postBlock); err != nil {
		c.logger.Error("failed to post block in governance manager", "err", err)
	}

	if isEndOfEpoch {
		if epoch, err = c.restartEpoch(fullBlock.Block.Header, dbTx); err != nil {
			c.logger.Error("failed to restart epoch after block inserted", "error", err)

			return
		}
	}

	if err := c.state.insertLastProcessedEventsBlock(fullBlock.Block.Number(), dbTx); err != nil {
		c.logger.Error("failed to update the last processed events block in db", "error", err)

		return
	}

	// commit DB transaction
	if err := dbTx.Commit(); err != nil {
		c.logger.Error("failed to commit transaction on PostBlock",
			"block", fullBlock.Block.Number(), "error", err)

		return
	}

	// finally update runtime state (lastBuiltBlock, epoch, proposerSnapshot)
	c.epoch = epoch
	c.lastBuiltBlock = fullBlock.Block.Header

	endTime := time.Now().UTC()

	c.logger.Debug("OnBlockInserted finished", "elapsedTime", endTime.Sub(startTime),
		"epoch", epoch.Number, "block", fullBlock.Block.Number())
}

// FSM creates a new instance of fsm
func (c *consensusRuntime) FSM() error {
	sharedData, err := c.getGuardedData()
	if err != nil {
		return fmt.Errorf("cannot create fsm: %w", err)
	}

	parent, epoch, proposerSnapshot := sharedData.lastBuiltBlock, sharedData.epoch, sharedData.proposerSnapshot

	if !epoch.Validators.ContainsNodeID(c.config.Key.String()) {
		return errNotAValidator
	}

	blockBuilder, err := c.config.blockchain.NewBlockBuilder(
		parent,
		types.Address(c.config.Key.Address()),
		c.config.txPool,
		epoch.CurrentClientConfig.BlockTime.Duration,
		c.logger,
	)

	if err != nil {
		return fmt.Errorf("cannot create block builder for fsm: %w", err)
	}

	pendingBlockNumber := parent.Number + 1
	// calculation of epoch and sprint end does not consider slashing currently
	isEndOfSprint := c.isFixedSizeOfSprintMet(pendingBlockNumber, epoch)
	isEndOfEpoch := c.isFixedSizeOfEpochMet(pendingBlockNumber, epoch)
	isFirstBlockOfEpoch := pendingBlockNumber == epoch.FirstBlockInEpoch

	valSet := validator.NewValidatorSet(epoch.Validators, c.logger)

	exitRootHash, err := c.bridgeManager.BuildExitEventRoot(epoch.Number)
	if err != nil {
		return fmt.Errorf("could not build exit root hash for fsm: %w", err)
	}

	ff := &fsm{
		config:              epoch.CurrentClientConfig,
		forks:               c.config.Forks,
		parent:              parent,
		backend:             c.config.blockchain,
		polybftBackend:      c.config.polybftBackend,
		exitEventRootHash:   exitRootHash,
		epochNumber:         epoch.Number,
		blockBuilder:        blockBuilder,
		validators:          valSet,
		isEndOfEpoch:        isEndOfEpoch,
		isEndOfSprint:       isEndOfSprint,
		isFirstBlockOfEpoch: isFirstBlockOfEpoch,
		proposerSnapshot:    proposerSnapshot,
		logger:              c.logger.Named("fsm"),
	}

	if isEndOfSprint {
		commitment, err := c.bridgeManager.Commitment(pendingBlockNumber)
		if err != nil {
			return err
		}

		ff.proposerCommitmentToRegister = commitment
	}

	if isEndOfEpoch {
		ff.commitEpochInput = createCommitEpochInput(parent, epoch)

		ff.newValidatorsDelta, err = c.stakeManager.UpdateValidatorSet(epoch.Number,
			epoch.CurrentClientConfig.MaxValidatorSetSize, epoch.Validators.Copy())
		if err != nil {
			return fmt.Errorf("cannot update validator set on epoch ending: %w", err)
		}
	}

	ff.distributeRewardsInput, err = c.calculateDistributeRewardsInput(isFirstBlockOfEpoch, isEndOfEpoch,
		pendingBlockNumber, parent, epoch.Number)
	if err != nil {
		return fmt.Errorf("cannot calculate uptime info: %w", err)
	}

	c.logger.Info(
		"[FSM built]",
		"epoch", epoch.Number,
		"endOfEpoch", isEndOfEpoch,
		"endOfSprint", isEndOfSprint,
	)

	c.lock.Lock()
	c.fsm = ff
	c.lock.Unlock()

	return nil
}

// restartEpoch resets the previously run epoch and moves to the next one
// returns *epochMetadata different from nil if the lastEpoch is not the current one and everything was successful
func (c *consensusRuntime) restartEpoch(header *types.Header, dbTx *bolt.Tx) (*epochMetadata, error) {
	lastEpoch := c.epoch

	systemState, err := c.getSystemState(header)
	if err != nil {
		return nil, fmt.Errorf("get system state: %w", err)
	}

	epochNumber, err := systemState.GetEpoch()
	if err != nil {
		return nil, fmt.Errorf("get epoch: %w", err)
	}

	if lastEpoch != nil {
		// Epoch might be already in memory, if its the same number do nothing -> just return provided last one
		// Otherwise, reset the epoch metadata and restart the async services
		if lastEpoch.Number == epochNumber {
			return lastEpoch, nil
		}
	}

	validatorSet, err := c.config.polybftBackend.GetValidatorsWithTx(header.Number, nil, dbTx)
	if err != nil {
		return nil, fmt.Errorf("restart epoch - cannot get validators: %w", err)
	}

	updateEpochMetrics(epochMetadata{
		Number:     epochNumber,
		Validators: validatorSet,
	})

	firstBlockInEpoch, err := c.getFirstBlockOfEpoch(epochNumber, header)
	if err != nil {
		return nil, err
	}

	if err := c.state.EpochStore.cleanEpochsFromDB(dbTx); err != nil {
		c.logger.Error("Could not clean previous epochs from db.", "error", err)
	}

	if err := c.state.EpochStore.insertEpoch(epochNumber, dbTx); err != nil {
		return nil, fmt.Errorf("an error occurred while inserting new epoch in db. Reason: %w", err)
	}

	c.logger.Info(
		"restartEpoch",
		"block number", header.Number,
		"epoch", epochNumber,
		"validators", validatorSet.Len(),
		"firstBlockInEpoch", firstBlockInEpoch,
	)

	reqObj := &PostEpochRequest{
		SystemState:       systemState,
		NewEpochID:        epochNumber,
		FirstBlockOfEpoch: firstBlockInEpoch,
		ValidatorSet:      validator.NewValidatorSet(validatorSet, c.logger),
		DBTx:              dbTx,
		Forks:             c.config.Forks,
	}

	if err := c.bridgeManager.PostEpoch(reqObj); err != nil {
		return nil, err
	}

	if err := c.governanceManager.PostEpoch(reqObj); err != nil {
		return nil, err
	}

	currentParams, err := c.governanceManager.GetClientConfig(dbTx)
	if err != nil {
		return nil, err
	}

	currentPolyConfig, err := GetPolyBFTConfig(currentParams)
	if err != nil {
		return nil, err
	}

	return &epochMetadata{
		Number:              epochNumber,
		Validators:          validatorSet,
		FirstBlockInEpoch:   firstBlockInEpoch,
		CurrentClientConfig: &currentPolyConfig,
	}, nil
}

// createCommitEpochInput creates commit epoch input data
func createCommitEpochInput(
	currentBlock *types.Header, epoch *epochMetadata) *contractsapi.CommitEpochEpochManagerFn {
	return &contractsapi.CommitEpochEpochManagerFn{
		ID: new(big.Int).SetUint64(epoch.Number),
		Epoch: &contractsapi.Epoch{
			StartBlock: new(big.Int).SetUint64(epoch.FirstBlockInEpoch),
			EndBlock:   new(big.Int).SetUint64(currentBlock.Number + 1),
			EpochRoot:  types.Hash{},
		},
		EpochSize: new(big.Int).SetUint64(epoch.CurrentClientConfig.EpochSize),
	}
}

// calculateDistributeRewardsInput calculates distribute rewards input data
func (c *consensusRuntime) calculateDistributeRewardsInput(
	isFirstBlockOfEpoch, isEndOfEpoch bool,
	pendingBlockNumber uint64,
	lastFinalizedBlock *types.Header,
	epochID uint64,
) (*contractsapi.DistributeRewardForEpochManagerFn, error) {
	if !isRewardDistributionBlock(c.config.Forks, isFirstBlockOfEpoch, isEndOfEpoch, pendingBlockNumber) {
		// we don't have to distribute rewards at this block
		return nil, nil
	}

	var (
		// epoch size is the number of blocks that really happened
		// because of slashing, epochs might not have the configured number of blocks
		epochSize     = uint64(0)
		uptimeCounter = map[types.Address]int64{}
		blockHeader   = lastFinalizedBlock // start calculating from this block
	)

	if forkmanager.GetInstance().IsForkEnabled(chain.Governance, pendingBlockNumber) {
		// if governance is enabled, we are distributing rewards for previous epoch
		// at the beginning of a new epoch, so modify epochID
		epochID--
	}

	getSealersForBlock := func(blockExtra *Extra, validators validator.AccountSet) error {
		signers, err := validators.GetFilteredValidators(blockExtra.Parent.Bitmap)
		if err != nil {
			return err
		}

		for _, a := range signers.GetAddresses() {
			uptimeCounter[a]++
		}

		epochSize++

		return nil
	}

	blockExtra, err := GetIbftExtra(blockHeader.ExtraData)
	if err != nil {
		return nil, err
	}

	previousBlockHeader, previousBlockExtra, err := getBlockData(blockHeader.Number-1, c.config.blockchain)
	if err != nil {
		return nil, err
	}

	// calculate uptime starting from last block - 1 in epoch until first block in given epoch
	for previousBlockExtra.Checkpoint.EpochNumber == blockExtra.Checkpoint.EpochNumber {
		validators, err := c.config.polybftBackend.GetValidators(blockHeader.Number-1, nil)
		if err != nil {
			return nil, err
		}

		if err := getSealersForBlock(blockExtra, validators); err != nil {
			return nil, err
		}

		blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)
		if err != nil {
			return nil, err
		}

		previousBlockHeader, previousBlockExtra, err = getBlockData(previousBlockHeader.Number-1, c.config.blockchain)
		if err != nil {
			return nil, err
		}
	}

	lookbackSize := getLookbackSizeForRewardDistribution(c.config.Forks, pendingBlockNumber)

	// calculate uptime for blocks from previous epoch that were not processed in previous uptime
	// since we can not calculate uptime for the last block in epoch (because of parent signatures)
	if blockHeader.Number > lookbackSize {
		for i := uint64(0); i < lookbackSize; i++ {
			validators, err := c.config.polybftBackend.GetValidators(blockHeader.Number-2, nil)
			if err != nil {
				return nil, err
			}

			if err := getSealersForBlock(blockExtra, validators); err != nil {
				return nil, err
			}

			blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)
			if err != nil {
				return nil, err
			}
		}
	}

	// include the data in the uptime counter in a deterministic way
	addrSet := []types.Address{}

	for addr := range uptimeCounter {
		addrSet = append(addrSet, addr)
	}

	uptime := make([]*contractsapi.Uptime, len(addrSet))

	sort.Slice(addrSet, func(i, j int) bool {
		return bytes.Compare(addrSet[i][:], addrSet[j][:]) > 0
	})

	for i, addr := range addrSet {
		uptime[i] = &contractsapi.Uptime{
			Validator:    addr,
			SignedBlocks: new(big.Int).SetInt64(uptimeCounter[addr]),
		}
	}

	distributeRewards := &contractsapi.DistributeRewardForEpochManagerFn{
		EpochID:   new(big.Int).SetUint64(epochID),
		Uptime:    uptime,
		EpochSize: new(big.Int).SetUint64(epochSize),
	}

	return distributeRewards, nil
}

// GenerateExitProof generates proof of exit and is a bridge endpoint store function
func (c *consensusRuntime) GenerateExitProof(exitID uint64) (types.Proof, error) {
	return c.bridgeManager.GenerateProof(exitID, Exit)
}

// GetStateSyncProof returns the proof for the state sync
func (c *consensusRuntime) GetStateSyncProof(stateSyncID uint64) (types.Proof, error) {
	return c.bridgeManager.GenerateProof(stateSyncID, StateSync)
}

// setIsActiveValidator updates the activeValidatorFlag field
func (c *consensusRuntime) setIsActiveValidator(isActiveValidator bool) {
	c.activeValidatorFlag.Store(isActiveValidator)
}

// isActiveValidator indicates if node is in validator set or not
func (c *consensusRuntime) IsActiveValidator() bool {
	return c.activeValidatorFlag.Load()
}

// isFixedSizeOfEpochMet checks if epoch reached its end that was configured by its default size
// this is only true if no slashing occurred in the given epoch
func (c *consensusRuntime) isFixedSizeOfEpochMet(blockNumber uint64, epoch *epochMetadata) bool {
	return epoch.FirstBlockInEpoch+epoch.CurrentClientConfig.EpochSize-1 == blockNumber
}

// isFixedSizeOfSprintMet checks if an end of an sprint is reached with the current block
func (c *consensusRuntime) isFixedSizeOfSprintMet(blockNumber uint64, epoch *epochMetadata) bool {
	return (blockNumber-epoch.FirstBlockInEpoch+1)%epoch.CurrentClientConfig.SprintSize == 0
}

// getSystemState builds SystemState instance for the most current block header
func (c *consensusRuntime) getSystemState(header *types.Header) (SystemState, error) {
	provider, err := c.config.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	return c.config.blockchain.GetSystemState(provider), nil
}

func (c *consensusRuntime) IsValidProposal(rawProposal []byte) bool {
	if err := c.fsm.Validate(rawProposal); err != nil {
		c.logger.Error("failed to validate proposal", "error", err)

		return false
	}

	return true
}

func (c *consensusRuntime) IsValidValidator(msg *proto.Message) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm == nil {
		c.logger.Warn("unable to validate IBFT message sender, because FSM is not initialized")

		return false
	}

	if err := c.fsm.ValidateSender(msg); err != nil {
		c.logger.Error("invalid IBFT message received", "error", err)

		return false
	}

	return true
}

func (c *consensusRuntime) IsProposer(id []byte, height, round uint64) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	nextProposer, err := c.fsm.proposerSnapshot.CalcProposer(round, height)
	if err != nil {
		c.logger.Error("cannot calculate proposer", "error", err)

		return false
	}

	c.logger.Info("Proposer calculated", "height", height, "round", round, "address", nextProposer)

	return bytes.Equal(id, nextProposer[:])
}

func (c *consensusRuntime) IsValidProposalHash(proposal *proto.Proposal, hash []byte) bool {
	if len(proposal.RawProposal) == 0 {
		c.logger.Error("proposal hash is not valid because proposal is empty")

		return false
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(proposal.RawProposal); err != nil {
		c.logger.Error("unable to unmarshal proposal", "error", err)

		return false
	}

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		c.logger.Error("failed to retrieve extra", "block number", block.Number(), "error", err)

		return false
	}

	proposalHash, err := extra.Checkpoint.Hash(c.config.blockchain.GetChainID(), block.Number(), block.Hash())
	if err != nil {
		c.logger.Error("failed to calculate proposal hash", "block number", block.Number(), "error", err)

		return false
	}

	return bytes.Equal(proposalHash.Bytes(), hash)
}

func (c *consensusRuntime) IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool {
	err := c.fsm.ValidateCommit(committedSeal.Signer, committedSeal.Signature, proposalHash)
	if err != nil {
		c.logger.Info("Invalid committed seal", "error", err)

		return false
	}

	return true
}

func (c *consensusRuntime) BuildProposal(view *proto.View) []byte {
	sharedData, err := c.getGuardedData()
	if err != nil {
		c.logger.Error("unable to build proposal", "error", err)

		return nil
	}

	if sharedData.lastBuiltBlock.Number+1 != view.Height {
		c.logger.Error("unable to build proposal, due to lack of parent block",
			"parent height", sharedData.lastBuiltBlock.Number, "current height", view.Height)

		return nil
	}

	proposal, err := c.fsm.BuildProposal(view.Round)
	if err != nil {
		c.logger.Error("unable to build proposal", "blockNumber", view, "error", err)

		return nil
	}

	return proposal
}

// InsertProposal inserts a proposal with the specified committed seals
func (c *consensusRuntime) InsertProposal(proposal *proto.Proposal, committedSeals []*messages.CommittedSeal) {
	fsm := c.fsm

	fullBlock, err := fsm.Insert(proposal.RawProposal, committedSeals)
	if err != nil {
		c.logger.Error("cannot insert proposal", "error", err)

		return
	}

	c.OnBlockInserted(fullBlock)
}

// ID return ID (address actually) of the current node
func (c *consensusRuntime) ID() []byte {
	return c.config.Key.Address().Bytes()
}

// GetVotingPowers returns map of validators addresses and their voting powers for the specified height.
func (c *consensusRuntime) GetVotingPowers(height uint64) (map[string]*big.Int, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if c.fsm == nil {
		return nil, errors.New("getting voting power failed - backend is not initialized")
	} else if c.fsm.Height() != height {
		return nil, fmt.Errorf("getting voting power failed - backend is not initialized for height %d, fsm height %d",
			height, c.fsm.Height())
	}

	return c.fsm.validators.GetVotingPowers(), nil
}

// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in proposal
func (c *consensusRuntime) BuildPrePrepareMessage(
	rawProposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	if len(rawProposal) == 0 {
		c.logger.Error("can not build pre-prepare message, since proposal is empty")

		return nil
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(rawProposal); err != nil {
		c.logger.Error(fmt.Sprintf("cannot unmarshal RLP: %s", err))

		return nil
	}

	extra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		c.logger.Error("failed to retrieve extra for block %d: %w", block.Number(), err)

		return nil
	}

	proposalHash, err := extra.Checkpoint.Hash(c.config.blockchain.GetChainID(), block.Number(), block.Hash())
	if err != nil {
		c.logger.Error("failed to calculate proposal hash", "block number", block.Number(), "error", err)

		return nil
	}

	proposal := &proto.Proposal{
		RawProposal: rawProposal,
		Round:       view.Round,
	}

	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: proposalHash.Bytes(),
				Certificate:  certificate,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "error", err)

		return nil
	}

	return message
}

// BuildPrepareMessage builds a PREPARE message based on the passed in proposal
func (c *consensusRuntime) BuildPrepareMessage(proposalHash []byte, view *proto.View) *proto.Message {
	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_PREPARE,
		Payload: &proto.Message_PrepareData{
			PrepareData: &proto.PrepareMessage{
				ProposalHash: proposalHash,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message.", "error", err)

		return nil
	}

	return message
}

// BuildCommitMessage builds a COMMIT message based on the passed in proposal
func (c *consensusRuntime) BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message {
	committedSeal, err := c.config.Key.SignWithDomain(proposalHash, signer.DomainCheckpointManager)
	if err != nil {
		c.logger.Error("Cannot create committed seal message.", "error", err)

		return nil
	}

	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_COMMIT,
		Payload: &proto.Message_CommitData{
			CommitData: &proto.CommitMessage{
				ProposalHash:  proposalHash,
				CommittedSeal: committedSeal,
			},
		},
	}

	message, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return message
}

// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in proposal
func (c *consensusRuntime) BuildRoundChangeMessage(
	proposal *proto.Proposal,
	certificate *proto.PreparedCertificate,
	view *proto.View,
) *proto.Message {
	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{
			RoundChangeData: &proto.RoundChangeMessage{
				LastPreparedProposal:      proposal,
				LatestPreparedCertificate: certificate,
			}},
	}

	signedMsg, err := c.config.Key.SignIBFTMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return signedMsg
}

// getFirstBlockOfEpoch returns the first block of epoch in which provided header resides
func (c *consensusRuntime) getFirstBlockOfEpoch(epochNumber uint64, latestHeader *types.Header) (uint64, error) {
	if latestHeader.Number == 0 {
		// if we are starting the chain, we know that the first block is block 1
		return 1, nil
	}

	blockHeader := latestHeader

	blockExtra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return 0, err
	}

	if epochNumber != blockExtra.Checkpoint.EpochNumber {
		// its a regular epoch ending. No out of sync happened
		return latestHeader.Number + 1, nil
	}

	// node was out of sync, so we need to figure out what was the first block of the given epoch
	epoch := blockExtra.Checkpoint.EpochNumber

	var firstBlockInEpoch uint64

	for blockExtra.Checkpoint.EpochNumber == epoch {
		firstBlockInEpoch = blockHeader.Number
		blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)

		if err != nil {
			return 0, err
		}
	}

	return firstBlockInEpoch, nil
}

// getCurrentBlockTimeDrift returns current block time drift
func (c *consensusRuntime) getCurrentBlockTimeDrift() uint64 {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.epoch.CurrentClientConfig.BlockTimeDrift
}

// getSealersForBlock checks who sealed a given block and updates the counter
func getSealersForBlock(sealersCounter map[types.Address]uint64,
	blockExtra *Extra, validators validator.AccountSet) error {
	signers, err := validators.GetFilteredValidators(blockExtra.Parent.Bitmap)
	if err != nil {
		return err
	}

	for _, a := range signers.GetAddresses() {
		sealersCounter[a]++
	}

	return nil
}
