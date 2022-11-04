package polybft

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"

	"github.com/0xPolygon/polygon-edge/blockchain"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/types"
	hcf "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

const (
	eventsBufferSize   = 10
	stateFileName      = "consensusState.db"
	uptimeLookbackSize = 2 // number of blocks to calculate uptime from the previous epoch
)

var (
	// state sync metrics
	// TODO: Nemanja - accommodate bridge metrics to Edge
	// totalStateSyncsMeter = metrics.NewRegisteredMeter("consensus/bridge/stateSyncsTotal", nil)

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

	// LastCheckpoint is the last epoch that was checkpointed, for now it is epoch-1.
	LastCheckpoint uint64

	// CheckpointProposer is the validator that has to send the checkpoint, assume it is static for now.
	CheckpointProposer string

	// Blocks is the list of blocks that we have to checkpoint in rootchain
	Blocks []*types.Block

	// Validators is the set of validators for the epoch
	Validators AccountSet

	// Commitment built in the current epoch
	Commitment *Commitment
}

// runtimeConfig is a struct that holds configuration data for given consensus runtime
type runtimeConfig struct {
	PolyBFTConfig   *PolyBFTConfig
	DataDir         string
	BridgeTransport BridgeTransport
	Key             *wallet.Key
	State           *State
	blockchain      blockchainBackend
	polybftBackend  polybftBackend
	txPool          txPoolInterface
}

// consensusRuntime is a struct that provides consensus runtime features like epoch, state and event management
type consensusRuntime struct {
	// config represents wrapper around required parameters which are received from the outside
	config *runtimeConfig

	// state is reference to the struct which encapsulates bridge events persistence logic
	state *State

	fsm *fsm

	// eventTracker is a reference to the log event tracker
	eventTracker *eventTracker

	// lock is a lock to access 'epoch' and `lastBuiltBlock`
	lock sync.RWMutex

	// epoch is the metadata for the current epoch
	epoch *epochMetadata

	// lastBuiltBlock is the header of the last processed block
	lastBuiltBlock *types.Header

	// activeValidatorFlag indicates whether the given node is amongst currently active validator set
	activeValidatorFlag uint32

	logger hcf.Logger
}

// newConsensusRuntime creates and starts a new consensus runtime instance with event tracking
func newConsensusRuntime(log hcf.Logger, config *runtimeConfig) *consensusRuntime {
	runtime := &consensusRuntime{
		state:  config.State,
		config: config,
		logger: log.Named("consensus_runtime"),
	}

	return runtime
}

// getEpoch returns current epochMetadata in a thread-safe manner.
func (c *consensusRuntime) getEpoch() *epochMetadata {
	c.lock.RLock()
	defer c.lock.RUnlock()

	return c.epoch
}

// getLastBuiltBlockAndEpoch returns last build block and current epochMetadata in a thread-safe manner.
func (c *consensusRuntime) getLastBuiltBlockAndEpoch() (*types.Header, *epochMetadata) {
	c.lock.RLock()
	defer c.lock.RUnlock()

	var lastBuiltBlock *types.Header
	if c.lastBuiltBlock != nil {
		lastBuiltBlock = new(types.Header)
		*lastBuiltBlock = *c.lastBuiltBlock
	}

	var epoch *epochMetadata
	if c.epoch != nil {
		epoch = new(epochMetadata)
		*epoch = *c.epoch
	}

	return lastBuiltBlock, epoch
}

func (c *consensusRuntime) IsBridgeEnabled() bool {
	return c.config.PolyBFTConfig.IsBridgeEnabled()
}

// AddLog is an implementation of eventSubscription interface,
// and is called from the event tracker when an event is final on the rootchain
func (c *consensusRuntime) AddLog(eventLog *ethgo.Log) {
	c.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	event, err := decodeEvent(eventLog)
	if err != nil {
		c.logger.Error("failed to decode state sync event", "hash", eventLog.TransactionHash, "err", err)

		return
	}

	if err := c.state.insertStateSyncEvent(event); err != nil {
		c.logger.Error("failed to insert state sync event", "hash", eventLog.TransactionHash, "err", err)

		return
	}
	// TODO: Nemanja
	// update metrics
	// totalStateSyncsMeter.Mark(1)
	return // TODO: Delete this when metrics is established. This is added just to trick linter.
}

// OnBlockInserted is called whenever fsm or syncer inserts new block
func (c *consensusRuntime) OnBlockInserted(block *types.Block) {
	// after the block has been written we reset the txpool so that the old transactions are removed
	c.config.txPool.ResetWithHeaders(block.Header)

	if c.isEndOfEpoch(block.Header.Number) {
		// reset the epoch. Internally it updates the parent block header.
		if err := c.restartEpoch(block.Header); err != nil {
			c.logger.Error("failed to restart epoch after block inserted", "err", err)
		}
	} else {
		c.lock.Lock()
		c.lastBuiltBlock = block.Header
		c.lock.Unlock()
	}
}

func (c *consensusRuntime) populateFsmIfBridgeEnabled(
	ff *fsm,
	epoch *epochMetadata,
	lastBuiltBlock *types.Header,
	isEndOfEpoch,
	isEndOfSprint bool,
) error {
	systemState, err := c.getSystemState(lastBuiltBlock)
	if err != nil {
		return err
	}

	nextStateSyncExecutionIdx, err := systemState.GetNextExecutionIndex()
	if err != nil {
		return fmt.Errorf("cannot get next execution index: %w", err)
	}

	ff.stateSyncExecutionIndex = nextStateSyncExecutionIdx

	nextRegisteredCommitmentIndex, err := systemState.GetNextCommittedIndex()
	if err != nil {
		return fmt.Errorf("cannot get next committed index: %w", err)
	}

	if isEndOfEpoch {
		commitment, err := c.getCommitmentToRegister(epoch, nextRegisteredCommitmentIndex)
		if err != nil {
			if errors.Is(err, ErrCommitmentNotBuilt) {
				c.logger.Debug(
					"[FSM] Have no built commitment to register",
					"epoch", epoch.Number,
					"from state sync index", nextRegisteredCommitmentIndex,
				)
			} else if errors.Is(err, errQuorumNotReached) {
				c.logger.Debug(
					"[FSM] Not enough votes to register commitment",
					"epoch", epoch.Number,
					"from state sync index", nextRegisteredCommitmentIndex,
				)
			} else {
				return fmt.Errorf("cannot get commitment to register: %w", err)
			}
		}

		ff.proposerCommitmentToRegister = commitment
	}

	if isEndOfSprint {
		if err := c.state.cleanCommitments(nextStateSyncExecutionIdx); err != nil {
			return fmt.Errorf("cannot clean commitments: %w", err)
		}

		nonExecutedCommitments, err := c.state.getNonExecutedCommitments(nextStateSyncExecutionIdx)
		if err != nil {
			return fmt.Errorf("cannot get non executed commitments: %w", err)
		}

		if len(nonExecutedCommitments) > 0 {
			bundlesToExecute, err := c.state.getBundles(nextStateSyncExecutionIdx, maxBundlesPerSprint)
			if err != nil {
				return fmt.Errorf("cannot get bundles: %w", err)
			}

			ff.commitmentsToVerifyBundles = nonExecutedCommitments
			ff.bundleProofs = bundlesToExecute
		}
	}

	return nil
}

// FSM creates a new instance of fsm
func (c *consensusRuntime) FSM() error {
	// figure out the parent. At this point this peer has done its best to sync up
	// to the head of their remote peers.
	parent, epoch := c.getLastBuiltBlockAndEpoch()

	if !epoch.Validators.ContainsNodeID(c.config.Key.NodeID()) {
		return errNotAValidator
	}

	blockBuilder, err := c.config.blockchain.NewBlockBuilder(
		parent,
		types.Address(c.config.Key.Address()),
		c.config.txPool,
		c.config.PolyBFTConfig.BlockTime,
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("cannot create block builder: %w", err)
	}

	pendingBlockNumber := parent.Number + 1
	isEndOfSprint := c.isEndOfSprint(pendingBlockNumber)
	isEndOfEpoch := c.isEndOfEpoch(pendingBlockNumber)

	ff := &fsm{
		config:         c.config.PolyBFTConfig,
		parent:         parent,
		backend:        c.config.blockchain,
		polybftBackend: c.config.polybftBackend,
		blockBuilder:   blockBuilder,
		validators:     newValidatorSet(types.BytesToAddress(parent.Miner), epoch.Validators),
		isEndOfEpoch:   isEndOfEpoch,
		isEndOfSprint:  isEndOfSprint,
		epoch:          epoch,
		logger:         c.logger.Named("fsm"),
	}

	if c.IsBridgeEnabled() {
		err := c.populateFsmIfBridgeEnabled(ff, epoch, parent, isEndOfEpoch, isEndOfSprint)
		if err != nil {
			return fmt.Errorf("cannot populate fsm: %w", err)
		}
	}

	if isEndOfEpoch {
		ff.uptimeCounter, err = c.calculateUptime(parent, epoch)
		if err != nil {
			return fmt.Errorf("cannot calculate uptime: %w", err)
		}
	}

	c.logger.Info(
		"[FSM built]",
		"epoch", epoch.Number,
		"endOfEpoch", isEndOfEpoch,
		"endOfSprint", isEndOfSprint,
	)

	c.fsm = ff

	return nil
}

// restartEpoch resets the previously run epoch and moves to the next one
func (c *consensusRuntime) restartEpoch(header *types.Header) error {
	systemState, err := c.getSystemState(header)
	if err != nil {
		return err
	}

	epochNumber, err := systemState.GetEpoch()
	if err != nil {
		return err
	}

	_, lastEpoch := c.getLastBuiltBlockAndEpoch()
	if lastEpoch != nil {
		// Epoch might be already in memory, if its the same number do nothing.
		// Otherwise, reset the epoch metadata and restart the async services
		if lastEpoch.Number == epochNumber {
			return nil
		}
	}

	/*
		// We will uncomment this once we have the clear PoC for the checkpoint
		lastCheckpoint := uint64(0)

		// get the blocks that should be signed for this checkpoint period
		blocks := []*types.Block{}

		epochSize := c.config.Config.PolyBFT.Epoch
		for i := lastCheckpoint * epochSize; i < epoch*epochSize; i++ {
			block := c.config.Blockchain.GetBlockByNumber(i)
			if block == nil {
				panic("block not found")
			} else {
				blocks = append(blocks, block)
			}
		}
	*/

	validatorSet, err := c.config.polybftBackend.GetValidators(header.Number, nil)
	if err != nil {
		return err
	}

	epoch := &epochMetadata{
		Number:         epochNumber,
		LastCheckpoint: 0,
		Blocks:         []*types.Block{},
		Validators:     validatorSet,
	}

	if err := c.state.cleanEpochsFromDB(); err != nil {
		c.logger.Error("Could not clean previous epochs from db.", "err", err)
	}

	if err := c.state.insertEpoch(epoch.Number); err != nil {
		return fmt.Errorf("an error occurred while inserting new epoch in db. Reason: %w", err)
	}

	// create commitment for state sync events
	if c.IsBridgeEnabled() {
		nextCommittedIndex, err := systemState.GetNextCommittedIndex()
		if err != nil {
			return err
		}

		commitment, err := c.buildCommitment(epochNumber, nextCommittedIndex)
		if err != nil {
			return err
		}

		epoch.Commitment = commitment
	}

	c.lock.Lock()
	c.epoch = epoch
	c.lastBuiltBlock = header
	c.lock.Unlock()

	err = c.runCheckpoint(epoch)
	if err != nil {
		return fmt.Errorf("could not run checkpoint: %w", err)
	}

	c.logger.Info(
		"restartEpoch",
		"block number", header.Number,
		"epoch", epochNumber,
		"validators", validatorSet.Len(),
	)

	return nil
}

// buildCommitment builds a commitment message (if it is not already built in previous epoch)
// for state sync events starting from given index and saves the message in database
func (c *consensusRuntime) buildCommitment(epoch, fromIndex uint64) (*Commitment, error) {
	toIndex := fromIndex + stateSyncMainBundleSize - 1
	// if it is not already built in the previous epoch
	stateSyncEvents, err := c.state.getStateSyncEventsForCommitment(fromIndex, toIndex)
	if err != nil {
		if errors.Is(err, ErrNotEnoughStateSyncs) {
			c.logger.Debug(
				"[buildCommitment] Not enough state syncs to build a commitment",
				"epoch", epoch,
				"from state sync index", fromIndex,
			)
			// this is a valid case, there is not enough state syncs
			return nil, nil
		}

		return nil, err
	}

	commitment, err := NewCommitment(epoch, fromIndex, toIndex, stateSyncBundleSize, stateSyncEvents)
	if err != nil {
		return nil, err
	}

	hash, err := commitment.Hash()
	if err != nil {
		return nil, err
	}

	hashBytes := hash.Bytes()
	signature, err := c.config.Key.Sign(hashBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      c.config.Key.NodeID(),
		Signature: signature,
	}

	if _, err = c.state.insertMessageVote(epoch, hashBytes, sig); err != nil {
		return nil, fmt.Errorf(
			"failed to insert signature for hash=%v to the state. Error: %w",
			hex.EncodeToString(hashBytes),
			err,
		)
	}

	// gossip message
	msg := &TransportMessage{
		Hash:        hashBytes,
		Signature:   signature,
		NodeID:      c.config.Key.NodeID(),
		EpochNumber: epoch,
	}
	c.config.BridgeTransport.Multicast(msg)

	c.logger.Debug(
		"[buildCommitment] Built commitment",
		"from", commitment.FromIndex,
		"to", commitment.ToIndex,
	)

	return commitment, nil
}

// buildBundles builds bundles if there is a created commitment by the validator and inserts them into db
func (c *consensusRuntime) buildBundles(commitment *Commitment, commitmentMsg *CommitmentMessage,
	stateSyncExecutionIndex uint64) error {
	c.logger.Debug(
		"[buildProofs] Building proofs...",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
		"nextExecutionIndex", stateSyncExecutionIndex,
	)

	if commitment == nil {
		// its a valid case when we do not have a built commitment so we can not build any proofs
		// we will be able to validate them though, since we have CommitmentMessageSigned taken from
		// register commitment state transaction when its block was inserted
		c.logger.Debug("[buildProofs] No commitment built.")

		return nil
	}

	var bundleProofs []*BundleProof

	startBundleIdx := commitmentMsg.GetBundleIdxFromStateSyncEventIdx(stateSyncExecutionIndex)

	for idx := startBundleIdx; idx < commitmentMsg.BundlesCount(); idx++ {
		p := commitment.MerkleTree.GenerateProof(idx, 0)
		events, err := c.getStateSyncEventsForBundle(commitmentMsg.GetFirstStateSyncIndexFromBundleIndex(idx),
			commitmentMsg.BundleSize)

		if err != nil {
			return err
		}

		bundleProofs = append(bundleProofs,
			&BundleProof{
				Proof:      p,
				StateSyncs: events,
			})
	}

	c.logger.Debug(
		"[buildProofs] Building proofs finished.",
		"fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex,
		"nextExecutionIndex", stateSyncExecutionIndex,
	)

	return c.state.insertBundles(bundleProofs)
}

// getAggSignatureForCommitmentMessage creates aggregated signatures for given commitment
// if it has a quorum of votes
func (c *consensusRuntime) getAggSignatureForCommitmentMessage(epoch *epochMetadata,
	commitmentHash types.Hash) (Signature, [][]byte, error) {
	validators := epoch.Validators

	nodeIDIndexMap := make(map[string]int, validators.Len())
	for i, validator := range validators {
		nodeIDIndexMap[validator.Address.String()] = i
	}

	// get all the votes from the database for this commitment
	votes, err := c.state.getMessageVotes(epoch.Number, commitmentHash.Bytes())
	if err != nil {
		return Signature{}, nil, err
	}

	var signatures bls.Signatures

	publicKeys := make([][]byte, 0)
	bitmap := bitmap.Bitmap{}

	for _, vote := range votes {
		index, exists := nodeIDIndexMap[vote.From]
		if !exists {
			continue // don't count this vote, because it does not belong to validator
		}

		signature, err := bls.UnmarshalSignature(vote.Signature)
		if err != nil {
			return Signature{}, nil, err
		}

		bitmap.Set(uint64(index))

		signatures = append(signatures, signature)
		publicKeys = append(publicKeys, validators[index].BlsKey.Marshal())
	}

	if len(signatures) < getQuorumSize(validators.Len()) {
		return Signature{}, nil, errQuorumNotReached
	}

	aggregatedSignature, err := signatures.Aggregate().Marshal()
	if err != nil {
		return Signature{}, nil, err
	}

	result := Signature{
		AggregatedSignature: aggregatedSignature,
		Bitmap:              bitmap,
	}

	return result, publicKeys, nil
}

// getStateSyncEventsForBundle gets state sync events from database for the appropriate bundle
func (c *consensusRuntime) getStateSyncEventsForBundle(from, bundleSize uint64) ([]*StateSyncEvent, error) {
	until := bundleSize + from - 1

	return c.state.getStateSyncEventsForCommitment(from, until)
}

// startEventTracker starts the event tracker that listens to state sync events
func (c *consensusRuntime) startEventTracker() error {
	if c.eventTracker != nil {
		return nil
	}

	c.eventTracker = &eventTracker{
		config:     c.config.PolyBFTConfig,
		subscriber: c,
		dataDir:    c.config.DataDir,
		logger:     c.logger.Named("event_tracker"),
	}

	if err := c.eventTracker.start(); err != nil {
		return err
	}

	return nil
}

// deliverMessage receives the message vote from transport and inserts it in state db for given epoch.
// It returns indicator whether message is processed successfully and error object if any.
func (c *consensusRuntime) deliverMessage(msg *TransportMessage) (bool, error) {
	_, epoch := c.getLastBuiltBlockAndEpoch()

	if epoch == nil || msg.EpochNumber < epoch.Number {
		// Epoch metadata is undefined
		// or received message for some of the older epochs.
		return false, nil
	}

	if !c.isActiveValidator() {
		return false, fmt.Errorf("validator is not among the active validator set")
	}

	// check just in case
	if epoch.Validators == nil {
		return false, fmt.Errorf("validators are not set for the current epoch")
	}

	msgVote := &MessageSignature{
		From:      msg.NodeID,
		Signature: msg.Signature,
	}

	if err := validateVote(msgVote, epoch); err != nil {
		return false, err
	}

	numSignatures, err := c.state.insertMessageVote(msg.EpochNumber, msg.Hash, msgVote)
	if err != nil {
		return false, fmt.Errorf("error inserting message vote: %w", err)
	}

	c.logger.Info(
		"deliver message",
		"hash", hex.EncodeToString(msg.Hash),
		"sender", msg.NodeID,
		"signatures", numSignatures,
		"quorum", getQuorumSize(len(epoch.Validators)),
	)

	return true, nil
}

func (c *consensusRuntime) runCheckpoint(epoch *epochMetadata) error {
	// TODO: Implement checkpoint
	return nil
}

// calculateUptime calculates uptime for blocks starting from the last built block in current epoch,
// and ending at the last block of previous epoch
func (c *consensusRuntime) calculateUptime(currentBlock *types.Header, epoch *epochMetadata) (*CommitEpoch, error) {
	uptimeCounter := map[types.Address]uint64{}

	if c.config.PolyBFTConfig.EpochSize < (uptimeLookbackSize + 1) {
		// this means that epoch size must at least be 3 blocks,
		// since we are not calculating uptime for lastBlockInEpoch and lastBlockInEpoch-1
		// they will be included in the uptime calculation of next epoch
		return nil, errors.New("epoch size not large enough to calculate uptime")
	}

	calculateUptimeForBlock := func(header *types.Header, validators AccountSet) error {
		blockExtra, err := GetIbftExtra(header.ExtraData)
		if err != nil {
			return err
		}

		signers, err := validators.GetFilteredValidators(blockExtra.Parent.Bitmap)
		if err != nil {
			return err
		}

		for _, a := range signers.GetAddresses() {
			uptimeCounter[a]++
		}

		return nil
	}

	firstBlockInEpoch := calculateFirstBlockOfPeriod(currentBlock.Number, c.config.PolyBFTConfig.EpochSize)
	lastBlockInPreviousEpoch := firstBlockInEpoch - 1

	startBlock := (epoch.Number * c.config.PolyBFTConfig.EpochSize) - c.config.PolyBFTConfig.EpochSize + 1
	endBlock := getEndEpochBlockNumber(epoch.Number, c.config.PolyBFTConfig.EpochSize)

	blockHeader := currentBlock
	validators := epoch.Validators

	var found bool

	for blockHeader.Number > firstBlockInEpoch {
		if err := calculateUptimeForBlock(blockHeader, validators); err != nil {
			return nil, err
		}

		blockHeader, found = c.config.blockchain.GetHeaderByNumber(blockHeader.Number - 1)
		if !found {
			return nil, blockchain.ErrNoBlock
		}
	}

	// since we need to calculate uptime for the last block of the previous epoch,
	// we need to get the validators for the that epoch from the smart contract
	// this is something that should probably be optimized
	if lastBlockInPreviousEpoch > 0 { // do not calculate anything for genesis block
		for i := 0; i < uptimeLookbackSize; i++ {
			validators, err := c.config.polybftBackend.GetValidators(blockHeader.Number-2, nil)
			if err != nil {
				return nil, err
			}

			if err := calculateUptimeForBlock(blockHeader, validators); err != nil {
				return nil, err
			}

			blockHeader, found = c.config.blockchain.GetHeaderByNumber(blockHeader.Number - 1)
			if !found {
				return nil, blockchain.ErrNoBlock
			}
		}
	}

	epochID := epoch.Number

	uptime := Uptime{EpochID: epochID}

	// include the data in the uptime counter in a deterministic way
	addrSet := []types.Address{}

	for addr := range uptimeCounter {
		addrSet = append(addrSet, addr)
	}

	sort.Slice(addrSet, func(i, j int) bool {
		return bytes.Compare(addrSet[i][:], addrSet[j][:]) > 0
	})

	for _, addr := range addrSet {
		uptime.addValidatorUptime(addr, uptimeCounter[addr])
	}

	commitEpoch := &CommitEpoch{
		EpochID: epochID,
		Epoch: Epoch{
			StartBlock: startBlock,
			EndBlock:   endBlock,
			EpochRoot:  types.Hash{},
		},
		Uptime: uptime,
	}

	return commitEpoch, nil
}

// setIsActiveValidator updates the activeValidatorFlag field
func (c *consensusRuntime) setIsActiveValidator(isActiveValidator bool) {
	if isActiveValidator {
		atomic.StoreUint32(&c.activeValidatorFlag, 1)
	} else {
		atomic.StoreUint32(&c.activeValidatorFlag, 0)
	}
}

// isActiveValidator indicates if node is in validator set or not
func (c *consensusRuntime) isActiveValidator() bool {
	return atomic.LoadUint32(&c.activeValidatorFlag) == 1
}

// isEndOfEpoch checks if an end of an epoch is reached with the current block
func (c *consensusRuntime) isEndOfEpoch(blockNumber uint64) bool {
	return isEndOfPeriod(blockNumber, c.config.PolyBFTConfig.EpochSize)
}

// isEndOfSprint checks if an end of an sprint is reached with the current block
func (c *consensusRuntime) isEndOfSprint(blockNumber uint64) bool {
	return isEndOfPeriod(blockNumber, c.config.PolyBFTConfig.SprintSize)
}

// getSystemState builds SystemState instance for the most current block header
func (c *consensusRuntime) getSystemState(header *types.Header) (SystemState, error) {
	provider, err := c.config.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	return c.config.blockchain.GetSystemState(c.config.PolyBFTConfig, provider), nil
}

// getCommitmentToRegister gets commitments to register via state transaction
func (c *consensusRuntime) getCommitmentToRegister(epoch *epochMetadata,
	registerCommitmentIndex uint64) (*CommitmentMessageSigned, error) {
	if epoch.Commitment == nil {
		// we did not build a commitment, so there is nothing to register
		return nil, ErrCommitmentNotBuilt
	}

	toIndex := registerCommitmentIndex + stateSyncMainBundleSize - 1
	commitmentMessage := NewCommitmentMessage(
		epoch.Commitment.MerkleTree.Hash(),
		registerCommitmentIndex,
		toIndex,
		stateSyncBundleSize)

	commitmentHash, err := epoch.Commitment.Hash()
	if err != nil {
		return nil, err
	}

	aggregatedSignature, publicKeys, err := c.getAggSignatureForCommitmentMessage(epoch, commitmentHash)
	if err != nil {
		return nil, err
	}

	return &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: aggregatedSignature,
		PublicKeys:   publicKeys,
	}, nil
}

func (c *consensusRuntime) IsValidBlock(proposal []byte) bool {
	if err := c.fsm.Validate(proposal); err != nil {
		c.logger.Error("failed validate proposal", "error", err)

		return false
	}

	return true
}

func (c *consensusRuntime) IsValidSender(msg *proto.Message) bool {
	err := c.fsm.ValidateSender(msg)
	if err != nil {
		c.logger.Error("invalid sender", "err", err)

		return false
	}

	return true
}

func (c *consensusRuntime) IsProposer(id []byte, height, round uint64) bool {
	nextProposer := c.fsm.validators.CalcProposer(round)

	return bytes.Equal(id, nextProposer[:])
}

func (c *consensusRuntime) IsValidProposalHash(proposal, hash []byte) bool {
	newBlock := types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		c.logger.Error("unable to unmarshal proposal", "err", err)

		return false
	}

	return bytes.Equal(newBlock.Header.Hash.Bytes(), hash)
}

func (c *consensusRuntime) IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool {
	err := c.fsm.ValidateCommit(committedSeal.Signer, committedSeal.Signature, proposalHash)
	if err != nil {
		c.logger.Info("Invalid committed seal", "err", err)

		return false
	}

	return true
}

func (c *consensusRuntime) BuildProposal(blockNumber uint64) []byte {
	lastBuiltBlock, _ := c.getLastBuiltBlockAndEpoch()

	if lastBuiltBlock.Number+1 != blockNumber {
		c.logger.Error("unable to build block, due to lack of parent block",
			"last", lastBuiltBlock.Number, "num", blockNumber)

		return nil
	}

	proposal, err := c.fsm.BuildProposal()
	if err != nil {
		c.logger.Info("Unable to create porposal", "blockNumber", blockNumber, "err", err)

		return nil
	}

	return proposal
}

func (c *consensusRuntime) InsertBlock(proposal []byte, committedSeals []*messages.CommittedSeal) {
	fsm := c.fsm
	if err := fsm.Insert(proposal, committedSeals); err != nil {
		c.logger.Error("cannot insert proposal", "err", err)

		return
	}

	if c.IsBridgeEnabled() {
		_, epoch := c.getLastBuiltBlockAndEpoch()

		if fsm.isEndOfEpoch && fsm.commitmentToSaveOnRegister != nil {
			if err := c.state.insertCommitmentMessage(fsm.commitmentToSaveOnRegister); err != nil {
				c.logger.Error("insert proposal, insert commitment message error", "err", err)
			}

			if err := c.buildBundles(
				epoch.Commitment, fsm.commitmentToSaveOnRegister.Message, fsm.stateSyncExecutionIndex); err != nil {
				c.logger.Error("insert proposal, build bundles error", "err", err)
			}
		}
	}

	c.OnBlockInserted(fsm.block.Block)
}

func (c *consensusRuntime) ID() []byte {
	return c.config.Key.Address().Bytes()
}

func (c *consensusRuntime) MaximumFaultyNodes() uint64 {
	return uint64(c.fsm.validators.Len()-1) / 3
}

func (c *consensusRuntime) Quorum(_ uint64) uint64 {
	return uint64(getQuorumSize(c.fsm.validators.Len()))
}

func (c *consensusRuntime) BuildPrePrepareMessage(
	proposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	block := types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
		c.logger.Error(fmt.Sprintf("cannot unmarshal RLP:%s", err))

		return nil
	}

	proposalHash := block.Hash().Bytes()

	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_PREPREPARE,
		Payload: &proto.Message_PreprepareData{
			PreprepareData: &proto.PrePrepareMessage{
				Proposal:     proposal,
				ProposalHash: proposalHash,
				Certificate:  certificate,
			},
		},
	}

	message, err := c.config.Key.SignEcdsaMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "error", err)

		return nil
	}

	return message
}

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

	message, err := c.config.Key.SignEcdsaMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message.", "error", err)

		return nil
	}

	return message
}

func (c *consensusRuntime) BuildCommitMessage(proposalHash []byte, view *proto.View) *proto.Message {
	committedSeal, err := c.config.Key.Sign(proposalHash)
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

	message, err := c.config.Key.SignEcdsaMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "error", err)

		return nil
	}

	return message
}

func (c *consensusRuntime) BuildRoundChangeMessage(
	proposal []byte,
	certificate *proto.PreparedCertificate,
	view *proto.View,
) *proto.Message {
	msg := proto.Message{
		View: view,
		From: c.ID(),
		Type: proto.MessageType_ROUND_CHANGE,
		Payload: &proto.Message_RoundChangeData{RoundChangeData: &proto.RoundChangeMessage{
			LastPreparedProposedBlock: proposal,
			LatestPreparedCertificate: certificate,
		}},
	}

	signedMsg, err := c.config.Key.SignEcdsaMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return signedMsg
}

// validateVote validates if the senders address is in active validator set
func validateVote(vote *MessageSignature, epoch *epochMetadata) error {
	// get senders address
	senderAddress := types.StringToAddress(vote.From)
	if !epoch.Validators.ContainsAddress(senderAddress) {
		return fmt.Errorf(
			"message is received from sender %s, which is not in current validator set",
			vote.From,
		)
	}

	return nil
}
