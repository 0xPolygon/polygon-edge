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
	"github.com/0xPolygon/pbft-consensus"
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

	// epoch is the metadata for the current epoch
	epoch *epochMetadata

	// lock is a lock to access 'epoch'
	lock sync.RWMutex

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
func (cr *consensusRuntime) getEpoch() *epochMetadata {
	cr.lock.RLock()
	defer cr.lock.RUnlock()

	return cr.epoch
}

func (cr *consensusRuntime) IsBridgeEnabled() bool {
	return cr.config.PolyBFTConfig.IsBridgeEnabled()
}

// AddLog is an implementation of eventSubscription interface,
// and is called from the event tracker when an event is final on the rootchain
func (cr *consensusRuntime) AddLog(eventLog *ethgo.Log) {
	cr.logger.Info(
		"Add State sync event",
		"block", eventLog.BlockNumber,
		"hash", eventLog.TransactionHash,
		"index", eventLog.LogIndex,
	)

	event, err := decodeEvent(eventLog)
	if err != nil {
		cr.logger.Error("failed to decode state sync event", "hash", eventLog.TransactionHash, "err", err)

		return
	}

	if err := cr.state.insertStateSyncEvent(event); err != nil {
		cr.logger.Error("failed to insert state sync event", "hash", eventLog.TransactionHash, "err", err)

		return
	}
	// TODO: Nemanja
	// update metrics
	// totalStateSyncsMeter.Mark(1)
	return // TODO: Delete this when metrics is established. This is added just to trick linter.
}

// OnBlockInserted is called whenever fsm or syncer inserts new block
func (cr *consensusRuntime) OnBlockInserted(block *types.Block) {
	// after the block has been written we reset the txpool so that the old transactions are removed
	cr.config.txPool.ResetWithHeaders(block.Header)

	if cr.isEndOfEpoch(block.Header.Number) {
		// reset the epoch. Internally it updates the parent block header.
		if err := cr.restartEpoch(block.Header); err != nil {
			cr.logger.Error("failed to restart epoch after block inserted", "err", err)
		}
	} else {
		cr.lock.Lock()
		cr.lastBuiltBlock = block.Header
		cr.lock.Unlock()
	}
}

func (cr *consensusRuntime) populateFsmIfBridgeEnabled(
	ff *fsm, epoch *epochMetadata, isEndOfEpoch, isEndOfSprint bool) error {
	systemState, err := cr.getSystemState(cr.lastBuiltBlock)
	if err != nil {
		return err
	}

	nextStateSyncExecutionIdx, err := systemState.GetNextExecutionIndex()
	if err != nil {
		return err
	}

	ff.stateSyncExecutionIndex = nextStateSyncExecutionIdx

	ff.postInsertHook = func() error {
		if isEndOfEpoch && ff.commitmentToSaveOnRegister != nil {
			if err := cr.state.insertCommitmentMessage(ff.commitmentToSaveOnRegister); err != nil {
				return err
			}

			if err := cr.buildBundles(
				epoch, ff.commitmentToSaveOnRegister.Message, nextStateSyncExecutionIdx); err != nil {
				return err
			}
		}

		cr.OnBlockInserted(ff.block.Block)

		return nil
	}

	nextRegisteredCommitmentIndex, err := systemState.GetNextCommittedIndex()
	if err != nil {
		return err
	}

	if isEndOfEpoch {
		commitment, err := cr.getCommitmentToRegister(epoch, nextRegisteredCommitmentIndex)
		if err != nil {
			if errors.Is(err, ErrCommitmentNotBuilt) {
				cr.logger.Debug("[FSM] Have no built commitment to register",
					"epoch", epoch.Number, "from state sync index", nextRegisteredCommitmentIndex)
			} else if errors.Is(err, errQuorumNotReached) {
				cr.logger.Debug("[FSM] Not enough votes to register commitment",
					"epoch", epoch.Number, "from state sync index", nextRegisteredCommitmentIndex)
			} else {
				return err
			}
		}

		ff.proposerCommitmentToRegister = commitment
	}

	if isEndOfSprint {
		if err := cr.state.cleanCommitments(nextStateSyncExecutionIdx); err != nil {
			return err
		}

		nonExecutedCommitments, err := cr.state.getNonExecutedCommitments(nextStateSyncExecutionIdx)
		if err != nil {
			return err
		}

		if len(nonExecutedCommitments) > 0 {
			bundlesToExecute, err := cr.state.getBundles(nextStateSyncExecutionIdx, maxBundlesPerSprint)
			if err != nil {
				return err
			}

			ff.commitmentsToVerifyBundles = nonExecutedCommitments
			ff.bundleProofs = bundlesToExecute
		}
	}

	return nil
}

// FSM creates a new instance of fsm
func (cr *consensusRuntime) FSM() error {
	// figure out the parent. At this point this peer has done its best to sync up
	// to the head of their remote peers.
	parent := cr.lastBuiltBlock
	epoch := cr.getEpoch()

	if !epoch.Validators.ContainsNodeID(cr.config.Key.NodeID()) {
		return errNotAValidator
	}

	blockBuilder, err := cr.config.blockchain.NewBlockBuilder(
		parent,
		types.Address(cr.config.Key.Address()),
		cr.config.txPool,
		cr.config.PolyBFTConfig.BlockTime,
		cr.logger,
	)
	if err != nil {
		return err
	}

	pendingBlockNumber := cr.getPendingBlockNumber()
	isEndOfSprint := cr.isEndOfSprint(pendingBlockNumber)
	isEndOfEpoch := cr.isEndOfEpoch(pendingBlockNumber)

	ff := &fsm{
		config:         cr.config.PolyBFTConfig,
		parent:         parent,
		backend:        cr.config.blockchain,
		polybftBackend: cr.config.polybftBackend,
		blockBuilder:   blockBuilder,
		validators:     newValidatorSet(types.BytesToAddress(parent.Miner), epoch.Validators),
		isEndOfEpoch:   isEndOfEpoch,
		isEndOfSprint:  isEndOfSprint,
		logger:         cr.logger.Named("fsm"),
	}

	if cr.IsBridgeEnabled() {
		err := cr.populateFsmIfBridgeEnabled(ff, epoch, isEndOfEpoch, isEndOfSprint)
		if err != nil {
			return err
		}
	} else {
		ff.postInsertHook = func() error {
			cr.OnBlockInserted(ff.block.Block)

			return nil
		}
	}

	if isEndOfEpoch {
		ff.uptimeCounter, err = cr.calculateUptime(parent)
		if err != nil {
			return err
		}
	}

	cr.logger.Info("[FSM built]",
		"epoch", epoch.Number,
		"endOfEpoch", isEndOfEpoch,
		"endOfSprint", isEndOfSprint,
	)

	cr.fsm = ff

	return nil
}

// restartEpoch resets the previously run epoch and moves to the next one
func (cr *consensusRuntime) restartEpoch(header *types.Header) error {
	systemState, err := cr.getSystemState(header)
	if err != nil {
		return err
	}

	epochNumber, err := systemState.GetEpoch()
	if err != nil {
		return err
	}

	lastEpoch := cr.getEpoch()
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

	validatorSet, err := cr.config.polybftBackend.GetValidators(header.Number, nil)
	if err != nil {
		return err
	}

	epoch := &epochMetadata{
		Number:         epochNumber,
		LastCheckpoint: 0,
		Blocks:         []*types.Block{},
		Validators:     validatorSet,
	}

	if err := cr.state.cleanEpochsFromDB(); err != nil {
		cr.logger.Error("Could not clean previous epochs from db.", "err", err)
	}

	if err := cr.state.insertEpoch(epoch.Number); err != nil {
		return fmt.Errorf("an error occurred while inserting new epoch in db. Reason: %w", err)
	}

	// create commitment for state sync events
	if cr.IsBridgeEnabled() {
		nextCommittedIndex, err := systemState.GetNextCommittedIndex()
		if err != nil {
			return err
		}

		commitment, err := cr.buildCommitment(epochNumber, nextCommittedIndex)
		if err != nil {
			return err
		}

		epoch.Commitment = commitment
	}

	cr.lock.Lock()
	cr.epoch = epoch
	cr.lastBuiltBlock = header
	cr.lock.Unlock()

	err = cr.runCheckpoint(epoch)
	if err != nil {
		return fmt.Errorf("could not run checkpoint:%w", err)
	}

	cr.logger.Info("restartEpoch", "block number", header.Number, "epoch", epochNumber, "validators", validatorSet)

	return nil
}

// buildCommitment builds a commitment message (if it is not already built in previous epoch)
// for state sync events starting from given index and saves the message in database
func (cr *consensusRuntime) buildCommitment(epoch, fromIndex uint64) (*Commitment, error) {
	toIndex := fromIndex + stateSyncMainBundleSize - 1
	// if it is not already built in the previous epoch
	stateSyncEvents, err := cr.state.getStateSyncEventsForCommitment(fromIndex, toIndex)
	if err != nil {
		if errors.Is(err, ErrNotEnoughStateSyncs) {
			cr.logger.Debug("[buildCommitment] Not enough state syncs to build a commitment",
				"epoch", epoch, "from state sync index", fromIndex)
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
	signature, err := cr.config.Key.Sign(hashBytes)

	if err != nil {
		return nil, fmt.Errorf("failed to sign commitment message. Error: %w", err)
	}

	sig := &MessageSignature{
		From:      cr.config.Key.NodeID(),
		Signature: signature,
	}

	if _, err = cr.state.insertMessageVote(epoch, hashBytes, sig); err != nil {
		return nil, fmt.Errorf("failed to insert signature for hash=%v to the state."+
			"Error: %v", hex.EncodeToString(hashBytes), err)
	}

	// gossip message
	msg := &TransportMessage{
		Hash:        hashBytes,
		Signature:   signature,
		NodeID:      cr.config.Key.NodeID(),
		EpochNumber: epoch,
	}
	cr.config.BridgeTransport.Multicast(msg)

	cr.logger.Debug("[buildCommitment] Built commitment", "from", commitment.FromIndex, "to", commitment.ToIndex)

	return commitment, nil
}

// buildBundles builds bundles if there is a created commitment by the validator and inserts them into db
func (cr *consensusRuntime) buildBundles(epoch *epochMetadata, commitmentMsg *CommitmentMessage,
	stateSyncExecutionIndex uint64) error {
	cr.logger.Debug("[buildProofs] Building proofs...", "fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex, "nextExecutionIndex", stateSyncExecutionIndex)

	if epoch.Commitment == nil {
		// its a valid case when we do not have a built commitment so we can not build any proofs
		// we will be able to validate them though, since we have CommitmentMessageSigned taken from
		// register commitment state transaction when its block was inserted
		cr.logger.Debug("[buildProofs] No commitment built.")

		return nil
	}

	var bundleProofs []*BundleProof
	startBundleIdx := commitmentMsg.GetBundleIdxFromStateSyncEventIdx(stateSyncExecutionIndex)

	for idx := startBundleIdx; idx < commitmentMsg.BundlesCount(); idx++ {
		p := epoch.Commitment.MerkleTree.GenerateProof(idx, 0)
		events, err := cr.getStateSyncEventsForBundle(commitmentMsg.GetFirstStateSyncIndexFromBundleIndex(idx),
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

	cr.logger.Debug("[buildProofs] Building proofs finished.", "fromIndex", commitmentMsg.FromIndex,
		"toIndex", commitmentMsg.ToIndex, "nextExecutionIndex", stateSyncExecutionIndex)

	return cr.state.insertBundles(bundleProofs)
}

// getAggSignatureForCommitmentMessage creates aggregated signatures for given commitment
// if it has a quorum of votes
func (cr *consensusRuntime) getAggSignatureForCommitmentMessage(epoch *epochMetadata,
	commitmentHash types.Hash) (Signature, [][]byte, error) {
	validators := epoch.Validators

	nodeIDIndexMap := make(map[string]int, validators.Len())
	for i, validator := range validators {
		nodeIDIndexMap[validator.Address.String()] = i
	}

	// get all the votes from the database for this commitment
	votes, err := cr.state.getMessageVotes(epoch.Number, commitmentHash.Bytes())
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
func (cr *consensusRuntime) getStateSyncEventsForBundle(from, bundleSize uint64) ([]*StateSyncEvent, error) {
	until := bundleSize + from - 1

	return cr.state.getStateSyncEventsForCommitment(from, until)
}

// startEventTracker starts the event tracker that listens to state sync events
func (cr *consensusRuntime) startEventTracker() error {
	if cr.eventTracker != nil {
		return nil
	}

	cr.eventTracker = &eventTracker{
		config:     cr.config.PolyBFTConfig,
		subscriber: cr,
		dataDir:    cr.config.DataDir,
		logger:     cr.logger.Named("event_tracker"),
	}

	if err := cr.eventTracker.start(); err != nil {
		return err
	}

	return nil
}

// deliverMessage receives the message vote from transport and inserts it in state db for given epoch.
// It returns indicator whether message is processed successfully and error object if any.
func (cr *consensusRuntime) deliverMessage(msg *TransportMessage) (bool, error) {
	epoch := cr.getEpoch()
	if epoch == nil || msg.EpochNumber < epoch.Number {
		// Epoch metadata is undefined
		// or received message for some of the older epochs.
		return false, nil
	}

	if !cr.isActiveValidator() {
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

	numSignatures, err := cr.state.insertMessageVote(msg.EpochNumber, msg.Hash, msgVote)
	if err != nil {
		return false, fmt.Errorf("error inserting message vote: %w", err)
	}

	cr.logger.Info(
		"deliver message",
		"hash", hex.EncodeToString(msg.Hash),
		"sender", msg.NodeID,
		"signatures", numSignatures,
		"quorum", getQuorumSize(len(epoch.Validators)),
	)

	return true, nil
}

func (cr *consensusRuntime) runCheckpoint(epoch *epochMetadata) error {
	// TODO: Implement checkpoint
	return nil
}

// getLatestSprintBlockNumber returns latest sprint block number
func (cr *consensusRuntime) getLatestSprintBlockNumber() uint64 {
	lastBuiltBlockNumber := cr.lastBuiltBlock.Number

	sprintSizeMod := lastBuiltBlockNumber % cr.config.PolyBFTConfig.SprintSize
	if sprintSizeMod == 0 {
		return lastBuiltBlockNumber
	}

	sprintBlockNumber := lastBuiltBlockNumber - sprintSizeMod

	return sprintBlockNumber
}

// calculateUptime calculates uptime for blocks starting from the last built block in current epoch,
// and ending at the last block of previous epoch
func (cr *consensusRuntime) calculateUptime(currentBlock *types.Header) (*CommitEpoch, error) {
	epoch := cr.getEpoch()
	uptimeCounter := map[types.Address]uint64{}

	if cr.config.PolyBFTConfig.EpochSize < (uptimeLookbackSize + 1) {
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

	firstBlockInEpoch := calculateFirstBlockOfPeriod(currentBlock.Number, cr.config.PolyBFTConfig.EpochSize)
	lastBlockInPreviousEpoch := firstBlockInEpoch - 1

	startBlock := (epoch.Number * cr.config.PolyBFTConfig.EpochSize) - cr.config.PolyBFTConfig.EpochSize + 1
	endBlock := getEndEpochBlockNumber(epoch.Number, cr.config.PolyBFTConfig.EpochSize)

	blockHeader := currentBlock
	validators := epoch.Validators

	var found bool

	for blockHeader.Number > firstBlockInEpoch {
		if err := calculateUptimeForBlock(blockHeader, validators); err != nil {
			return nil, err
		}

		blockHeader, found = cr.config.blockchain.GetHeaderByNumber(blockHeader.Number - 1)
		if !found {
			return nil, blockchain.ErrNoBlock
		}
	}

	// since we need to calculate uptime for the last block of the previous epoch,
	// we need to get the validators for the that epoch from the smart contract
	// this is something that should probably be optimized
	if lastBlockInPreviousEpoch > 0 { // do not calculate anything for genesis block
		for i := 0; i < uptimeLookbackSize; i++ {
			validators, err := cr.config.polybftBackend.GetValidators(blockHeader.Number-2, nil)
			if err != nil {
				return nil, err
			}

			if err := calculateUptimeForBlock(blockHeader, validators); err != nil {
				return nil, err
			}

			blockHeader, found = cr.config.blockchain.GetHeaderByNumber(blockHeader.Number - 1)
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
func (cr *consensusRuntime) setIsActiveValidator(isActiveValidator bool) {
	if isActiveValidator {
		atomic.StoreUint32(&cr.activeValidatorFlag, 1)
	} else {
		atomic.StoreUint32(&cr.activeValidatorFlag, 0)
	}
}

// isActiveValidator indicates if node is in validator set or not
func (cr *consensusRuntime) isActiveValidator() bool {
	return atomic.LoadUint32(&cr.activeValidatorFlag) == 1
}

// getPendingBlockNumber returns block number currently being built (last built block number + 1)
func (cr *consensusRuntime) getPendingBlockNumber() uint64 {
	return cr.lastBuiltBlock.Number + 1
}

// isEndOfEpoch checks if an end of an epoch is reached with the current block
func (cr *consensusRuntime) isEndOfEpoch(blockNumber uint64) bool {
	return isEndOfPeriod(blockNumber, cr.config.PolyBFTConfig.EpochSize)
}

// isEndOfSprint checks if an end of an sprint is reached with the current block
func (cr *consensusRuntime) isEndOfSprint(blockNumber uint64) bool {
	return isEndOfPeriod(blockNumber, cr.config.PolyBFTConfig.SprintSize)
}

// getSystemState builds SystemState instance for the most current block header
func (cr *consensusRuntime) getSystemState(header *types.Header) (SystemState, error) {
	provider, err := cr.config.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	return cr.config.blockchain.GetSystemState(cr.config.PolyBFTConfig, provider), nil
}

// getCommitmentToRegister gets commitments to register via state transaction
func (cr *consensusRuntime) getCommitmentToRegister(epoch *epochMetadata,
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

	aggregatedSignature, publicKeys, err := cr.getAggSignatureForCommitmentMessage(epoch, commitmentHash)
	if err != nil {
		return nil, err
	}

	return &CommitmentMessageSigned{
		Message:      commitmentMessage,
		AggSignature: aggregatedSignature,
		PublicKeys:   publicKeys,
	}, nil
}

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

// Implementation of core.Verifier

func (cr *consensusRuntime) IsValidBlock(proposal []byte) bool {
	var block types.Block
	if err := block.UnmarshalRLP(proposal); err != nil {
		cr.logger.Error("failed to decode block data", "error", err)

		return false
	}

	cr.logger.Debug("[FSM Validate]", "hash", block.Hash().String())

	// validate proposal
	// TODO do we need this?
	// if block.Hash() != types.BytesToHash(proposal.Hash()) {
	// 	return fmt.Errorf("incorrect sign hash (current header#%d)", block.Number())
	// }

	// validate header fields
	if err := validateHeaderFields(cr.fsm.parent, block.Header); err != nil {
		cr.logger.Error("failed to validate header",
			"parentHeader", cr.fsm.parent.Number,
			"parentBlock", cr.fsm.parent.Number,
			"block", block.Number(),
			"error", err)

		return false
	}

	blockExtra, err := GetIbftExtra(block.Header.ExtraData)
	if err != nil {
		cr.logger.Error("cannot get block extra data", "error", err)

		return false
	}

	// TODO: Validate validator set delta?

	blockNumber := block.Number()
	if blockNumber > 1 {
		// verify parent signature
		// We skip block 0 (genesis) and block 1 (parent is genesis)
		// since those blocks do not include any parent information with signatures
		validators, err := cr.fsm.polybftBackend.GetValidators(blockNumber-2, nil)
		if err != nil {
			cr.logger.Error("cannot get validators", "error", err)

			return false
		}

		cr.logger.Trace("[FSM Validate]", "Block", blockNumber, "parent validators", validators)
		parentHash := cr.fsm.parent.Hash

		if err := blockExtra.Parent.VerifyCommittedFields(validators, parentHash); err != nil {
			cr.logger.Error(
				"failed to verify signatures for (parent)",
				"parentBlock", cr.fsm.parent.Number,
				"parentHash", parentHash,
				"block", block,
				"error", err,
			)

			return false
		}
	}

	if err := cr.fsm.VerifyStateTransactions(block.Transactions); err != nil {
		cr.logger.Error("cannot verify state transactions", "error", err)

		return false
	}

	builtBlock, err := cr.fsm.backend.ProcessBlock(cr.fsm.parent, &block)
	if err != nil {
		cr.logger.Error("cannot process block", "error", err)

		return false
	}

	cr.fsm.block = builtBlock

	fsmProposal := pbft.Proposal{
		Data: proposal,
		// TODO add rest of the fields?
		// Time:
		// Hash:
	}

	cr.fsm.proposal = &fsmProposal

	cr.logger.Debug("[FSM Validate]",
		"txs", len(cr.fsm.block.Block.Transactions),
		"hash", block.Hash().String())

	return true
}

// IsValidSender checks if signature is from sender
func (cr *consensusRuntime) IsValidSender(msg *proto.Message) bool {
	// TODO it is always true
	return true
}

// IsProposer checks if the passed in ID is the Proposer for current view (sequence, round)
func (cr *consensusRuntime) IsProposer(id []byte, height, round uint64) bool {
	nextProposer := cr.fsm.validators.CalcProposer(round)

	return types.BytesToAddress(id) == types.StringToAddress(nextProposer)
}

// IsValidProposalHash checks if the hash matches the proposal
func (cr *consensusRuntime) IsValidProposalHash(proposal, hash []byte) bool {
	newBlock := &types.Block{}
	if err := newBlock.UnmarshalRLP(proposal); err != nil {
		cr.logger.Error("unable to unmarshal proposal", "err", err)

		return false
	}

	blockHash := newBlock.Header.Hash.Bytes()

	return bytes.Equal(blockHash, hash)
}

// IsValidCommittedSeal checks if the seal for the proposal is valid
func (cr *consensusRuntime) IsValidCommittedSeal(proposalHash []byte, committedSeal *messages.CommittedSeal) bool {
	from := types.BytesToAddress(committedSeal.Signer)
	err := cr.fsm.ValidateCommit(from, committedSeal.Signature, proposalHash)

	if err != nil {
		cr.logger.Info(
			"Invalid committed seal",
			"err", err,
		)

		return false
	}

	return true
}

// Implementation of core.Backend

func (cr *consensusRuntime) BuildProposal(blockNumber uint64) []byte {
	if cr.lastBuiltBlock.Number+1 != blockNumber {
		cr.logger.Error(
			"unable to build block, due to lack of parent block",
			"num",
			cr.lastBuiltBlock.Number,
		)

		return nil
	}

	proposal, err := cr.fsm.BuildProposal()

	if err != nil {
		cr.logger.Info(
			"Unable to create porposal",
			"blockNumber", blockNumber,
			"err", err,
		)

		return nil
	}

	return proposal.Data
}

func (cr *consensusRuntime) InsertBlock(proposal []byte, committedSeals []*messages.CommittedSeal) {
	// newBlock := &types.Block{}
	// if err := newBlock.UnmarshalRLP(proposal); err != nil {
	// 	cr.logger.Error("cannot unmarshal proposal", "err", err)

	// 	return
	// }

	// committedSealsMap := make(map[types.Address][]byte, len(committedSeals))

	// for _, cm := range committedSeals {
	// 	committedSealsMap[types.BytesToAddress(cm.Signer)] = cm.Signature
	// }

	// // Push the committed seals to the header
	// header, err := i.currentSigner.WriteCommittedSeals(newBlock.Header, committedSealsMap)
	// if err != nil {
	// 	i.logger.Error("cannot write committed seals", "err", err)

	// 	return
	// }

	// newBlock.Header = header

	// // Save the block locally
	// if err := i.blockchain.WriteBlock(newBlock, "consensus"); err != nil {
	// 	i.logger.Error("cannot write block", "err", err)

	// 	return
	// }

	// i.updateMetrics(newBlock)

	// i.logger.Info(
	// 	"block committed",
	// 	"number", newBlock.Number(),
	// 	"hash", newBlock.Hash(),
	// 	"validation_type", i.currentSigner.Type(),
	// 	"validators", i.currentValidators.Len(),
	// 	"committed", len(committedSeals),
	// )

	// if err := i.currentHooks.PostInsertBlock(newBlock); err != nil {
	// 	i.logger.Error(
	// 		"failed to call PostInsertBlock hook",
	// 		"height", newBlock.Number(),
	// 		"hash", newBlock.Hash(),
	// 		"err", err,
	// 	)

	// 	return
	// }

	// // after the block has been written we reset the txpool so that
	// // the old transactions are removed
	// i.txpool.ResetWithHeaders(newBlock.Header)

	// cr.fsm.Insert()
	panic("not implemented")
}

func (cr *consensusRuntime) ID() []byte {
	return cr.config.Key.Address().Bytes()
}

func (cr *consensusRuntime) MaximumFaultyNodes() uint64 {
	return uint64((len(cr.epoch.Validators) - 1) / 3)
}

func (cr *consensusRuntime) Quorum(validatorsCount uint64) uint64 {
	return uint64(getQuorumSize(int(validatorsCount)))
}
