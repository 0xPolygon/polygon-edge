package polybft

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"

	"github.com/0xPolygon/go-ibft/messages"
	"github.com/0xPolygon/go-ibft/messages/proto"
	hcf "github.com/hashicorp/go-hclog"
)

const (
	eventsBufferSize   = 10
	stateFileName      = "consensusState.db"
	uptimeLookbackSize = 2 // number of blocks to calculate uptime from the previous epoch
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

// checkpointBackend is an interface providing functions for working with checkpoints and exit evens
type checkpointBackend interface {
	// BuildEventRoot generates an event root hash from exit events in given epoch
	BuildEventRoot(epoch uint64) (types.Hash, error)
	// InsertExitEvents inserts provided exit events to persistence storage
	InsertExitEvents(exitEvents []*ExitEvent) error
}

// epochMetadata is the static info for epoch currently being processed
type epochMetadata struct {
	// Number is the number of the epoch
	Number uint64

	FirstBlockInEpoch uint64

	// Validators is the set of validators for the epoch
	Validators AccountSet
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
	PolyBFTConfig  *PolyBFTConfig
	DataDir        string
	Key            *wallet.Key
	State          *State
	blockchain     blockchainBackend
	polybftBackend polybftBackend
	txPool         txPoolInterface
	bridgeTopic    topic
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
	activeValidatorFlag uint32

	// checkpointManager represents abstraction for checkpoint submission
	checkpointManager *checkpointManager

	// proposerCalculator is the object which manipulates with ProposerSnapshot
	proposerCalculator *ProposerCalculator

	// manager for state sync bridge transactions
	stateSyncManager StateSyncManager

	// logger instance
	logger hcf.Logger
}

// newConsensusRuntime creates and starts a new consensus runtime instance with event tracking
func newConsensusRuntime(log hcf.Logger, config *runtimeConfig) (*consensusRuntime, error) {
	proposerCalculator, err := NewProposerCalculator(config, log.Named("proposer_calculator"))
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus runtime, error while creating proposer calculator %w", err)
	}

	runtime := &consensusRuntime{
		state:              config.State,
		config:             config,
		lastBuiltBlock:     config.blockchain.CurrentHeader(),
		proposerCalculator: proposerCalculator,
		logger:             log.Named("consensus_runtime"),
	}

	if err := runtime.initStateSyncManager(log); err != nil {
		return nil, err
	}

	if runtime.IsBridgeEnabled() {
		// enable checkpoint manager
		txRelayer, err := txrelayer.NewTxRelayer(txrelayer.WithIPAddress(config.PolyBFTConfig.Bridge.JSONRPCEndpoint))
		if err != nil {
			return nil, err
		}

		runtime.checkpointManager = newCheckpointManager(
			wallet.NewEcdsaSigner(config.Key),
			defaultCheckpointsOffset,
			config.PolyBFTConfig.Bridge.CheckpointAddr,
			txRelayer,
			config.blockchain,
			config.polybftBackend,
			log.Named("checkpoint_manager"))
	}

	// we need to call restart epoch on runtime to initialize epoch state
	runtime.epoch, err = runtime.restartEpoch(runtime.lastBuiltBlock)
	if err != nil {
		return nil, fmt.Errorf("consensus runtime creation - restart epoch failed: %w", err)
	}

	return runtime, nil
}

// initStateSyncManager initializes state sync manager
// if bridge is not enabled, then a dummy state sync manager will be used
func (c *consensusRuntime) initStateSyncManager(log hcf.Logger) error {
	if c.IsBridgeEnabled() {
		stateSyncManager, err := NewStateSyncManager(
			log,
			c.config.State,
			&stateSyncConfig{
				key:             c.config.Key,
				stateSenderAddr: c.config.PolyBFTConfig.Bridge.BridgeAddr,
				jsonrpcAddr:     c.config.PolyBFTConfig.Bridge.JSONRPCEndpoint,
				dataDir:         c.config.DataDir,
				topic:           c.config.bridgeTopic,
			},
		)

		if err != nil {
			return err
		}

		c.stateSyncManager = stateSyncManager
	} else {
		c.stateSyncManager = &dummyStateSyncManager{}
	}

	return c.stateSyncManager.Init()
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
	return c.config.PolyBFTConfig.IsBridgeEnabled()
}

// OnBlockInserted is called whenever fsm or syncer inserts new block
func (c *consensusRuntime) OnBlockInserted(block *types.Block) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.lastBuiltBlock != nil && c.lastBuiltBlock.Number >= block.Number() {
		c.logger.Debug("on block inserted already handled",
			"current", c.lastBuiltBlock.Number, "block", block.Number())

		return
	}

	if err := updateBlockMetrics(block, c.lastBuiltBlock); err != nil {
		c.logger.Error("failed to update block metrics", "error", err)
	}

	// after the block has been written we reset the txpool so that the old transactions are removed
	c.config.txPool.ResetWithHeaders(block.Header)

	// handle commitment and proofs creation
	if err := c.stateSyncManager.PostBlock(&PostBlockRequest{Block: block}); err != nil {
		c.logger.Error("failed to post block state sync", "err", err)
	}

	var (
		epoch = c.epoch
		err   error
	)

	// TODO - this condition will need to be changed to recognize that either slashing happened
	// or epoch reached its fixed size
	if c.isFixedSizeOfEpochMet(block.Header.Number, c.epoch) {
		if epoch, err = c.restartEpoch(block.Header); err != nil {
			c.logger.Error("failed to restart epoch after block inserted", "error", err)

			return
		}
	}

	if err := c.proposerCalculator.Update(block.Number()); err != nil {
		// do not return if proposer snapshot hasn't been inserted, next call of OnBlockInserted will catch-up
		c.logger.Warn("Could not update proposer calculator", "err", err)
	}

	// finally update runtime state (lastBuiltBlock, epoch, proposerSnapshot)
	c.epoch = epoch
	c.lastBuiltBlock = block.Header
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
		c.config.PolyBFTConfig.BlockTime,
		c.logger,
	)
	if err != nil {
		return fmt.Errorf("cannot create block builder for fsm: %w", err)
	}

	// TODO - recognize slashing occurred
	slash := false

	pendingBlockNumber := parent.Number + 1
	isEndOfSprint := slash || c.isFixedSizeOfSprintMet(pendingBlockNumber, epoch)
	isEndOfEpoch := slash || c.isFixedSizeOfEpochMet(pendingBlockNumber, epoch)

	valSet := NewValidatorSet(epoch.Validators, c.logger)

	ff := &fsm{
		config:            c.config.PolyBFTConfig,
		parent:            parent,
		backend:           c.config.blockchain,
		polybftBackend:    c.config.polybftBackend,
		checkpointBackend: c,
		epochNumber:       epoch.Number,
		blockBuilder:      blockBuilder,
		validators:        valSet,
		isEndOfEpoch:      isEndOfEpoch,
		isEndOfSprint:     isEndOfSprint,
		proposerSnapshot:  proposerSnapshot,
		logger:            c.logger.Named("fsm"),
	}

	commitment, err := c.stateSyncManager.Commitment()
	if err != nil {
		return err
	}

	ff.proposerCommitmentToRegister = commitment

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

	c.lock.Lock()
	c.fsm = ff
	c.lock.Unlock()

	return nil
}

// restartEpoch resets the previously run epoch and moves to the next one
// returns *epochMetadata different from nil if the lastEpoch is not the current one and everything was successful
func (c *consensusRuntime) restartEpoch(header *types.Header) (*epochMetadata, error) {
	lastEpoch := c.epoch

	systemState, err := c.getSystemState(header)
	if err != nil {
		return nil, err
	}

	epochNumber, err := systemState.GetEpoch()
	if err != nil {
		return nil, err
	}

	if lastEpoch != nil {
		// Epoch might be already in memory, if its the same number do nothing -> just return provided last one
		// Otherwise, reset the epoch metadata and restart the async services
		if lastEpoch.Number == epochNumber {
			return lastEpoch, nil
		}
	}

	validatorSet, err := c.config.polybftBackend.GetValidators(header.Number, nil)
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

	if err := c.state.cleanEpochsFromDB(); err != nil {
		c.logger.Error("Could not clean previous epochs from db.", "error", err)
	}

	if err := c.state.insertEpoch(epochNumber); err != nil {
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
		BlockNumber:  header.Number,
		SystemState:  systemState,
		NewEpochID:   epochNumber,
		ValidatorSet: NewValidatorSet(validatorSet, c.logger),
	}

	if err := c.stateSyncManager.PostEpoch(reqObj); err != nil {
		return nil, err
	}

	return &epochMetadata{
		Number:            epochNumber,
		Validators:        validatorSet,
		FirstBlockInEpoch: firstBlockInEpoch,
	}, nil
}

// calculateUptime calculates uptime for blocks starting from the last built block in current epoch,
// and ending at the last block of previous epoch
func (c *consensusRuntime) calculateUptime(currentBlock *types.Header, epoch *epochMetadata) (*CommitEpoch, error) {
	uptimeCounter := map[types.Address]uint64{}
	blockHeader := currentBlock
	epochID := epoch.Number
	totalBlocks := uint64(0)

	getSealersForBlock := func(blockExtra *Extra, validators AccountSet) error {
		signers, err := validators.GetFilteredValidators(blockExtra.Parent.Bitmap)
		if err != nil {
			return err
		}

		totalBlocks++

		for _, a := range signers.GetAddresses() {
			uptimeCounter[a]++
		}

		return nil
	}

	blockExtra, err := GetIbftExtra(currentBlock.ExtraData)
	if err != nil {
		return nil, err
	}

	// calculate uptime for current epoch
	for blockHeader.Number > epoch.FirstBlockInEpoch {
		if err := getSealersForBlock(blockExtra, epoch.Validators); err != nil {
			return nil, err
		}

		blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)
	}

	// calculate uptime for blocks from previous epoch that were not processed in previous uptime
	// since we can not calculate uptime for the last block in epoch (because of parent signatures)
	if blockHeader.Number > uptimeLookbackSize {
		for i := 0; i < uptimeLookbackSize; i++ {
			validators, err := c.config.polybftBackend.GetValidators(blockHeader.Number-2, nil)
			if err != nil {
				return nil, err
			}

			if err := getSealersForBlock(blockExtra, validators); err != nil {
				return nil, err
			}

			blockHeader, blockExtra, err = getBlockData(blockHeader.Number-1, c.config.blockchain)
		}
	}

	uptime := Uptime{
		EpochID:     epochID,
		TotalBlocks: totalBlocks,
	}

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
		EpochID: epoch.Number,
		Epoch: Epoch{
			StartBlock: epoch.FirstBlockInEpoch,
			EndBlock:   currentBlock.Number + 1,
			EpochRoot:  types.Hash{},
		},
		Uptime: uptime,
	}

	return commitEpoch, nil
}

// InsertExitEvents is an implementation of checkpointBackend interface
func (c *consensusRuntime) InsertExitEvents(exitEvents []*ExitEvent) error {
	return c.state.insertExitEvents(exitEvents)
}

// BuildEventRoot is an implementation of checkpointBackend interface
func (c *consensusRuntime) BuildEventRoot(epoch uint64) (types.Hash, error) {
	exitEvents, err := c.state.getExitEventsByEpoch(epoch)
	if err != nil {
		return types.ZeroHash, err
	}

	if len(exitEvents) == 0 {
		return types.ZeroHash, nil
	}

	tree, err := createExitTree(exitEvents)
	if err != nil {
		return types.ZeroHash, err
	}

	return tree.Hash(), nil
}

// GenerateExitProof generates proof of exit
func (c *consensusRuntime) GenerateExitProof(exitID, epoch, checkpointBlock uint64) (types.ExitProof, error) {
	exitEvent, err := c.state.getExitEvent(exitID, epoch)
	if err != nil {
		return types.ExitProof{}, err
	}

	e, err := ExitEventABIType.Encode(exitEvent)
	if err != nil {
		return types.ExitProof{}, err
	}

	exitEvents, err := c.state.getExitEventsForProof(epoch, checkpointBlock)
	if err != nil {
		return types.ExitProof{}, err
	}

	tree, err := createExitTree(exitEvents)
	if err != nil {
		return types.ExitProof{}, err
	}

	leafIndex, err := tree.LeafIndex(e)
	if err != nil {
		return types.ExitProof{}, err
	}

	proof, err := tree.GenerateProofForLeaf(e, 0)
	if err != nil {
		return types.ExitProof{}, err
	}

	return types.ExitProof{
		Proof:     proof,
		LeafIndex: leafIndex,
	}, nil
}

// GetStateSyncProof returns the proof for the state sync
func (c *consensusRuntime) GetStateSyncProof(stateSyncID uint64) (*types.StateSyncProof, error) {
	proof, err := c.state.getStateSyncProof(stateSyncID)
	if err != nil {
		return nil, fmt.Errorf("cannot get state sync proof for StateSync id %d: %w", stateSyncID, err)
	}

	if proof == nil {
		return nil, fmt.Errorf("cannot find state sync proof containing StateSync id %d", stateSyncID)
	}

	return proof, nil
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

// isFixedSizeOfEpochMet checks if epoch reached its end that was configured by its default size
// this is only true if no slashing occurred in the given epoch
func (c *consensusRuntime) isFixedSizeOfEpochMet(blockNumber uint64, epoch *epochMetadata) bool {
	return epoch.FirstBlockInEpoch+c.config.PolyBFTConfig.EpochSize-1 == blockNumber
}

// isFixedSizeOfSprintMet checks if an end of an sprint is reached with the current block
func (c *consensusRuntime) isFixedSizeOfSprintMet(blockNumber uint64, epoch *epochMetadata) bool {
	return (blockNumber-epoch.FirstBlockInEpoch+1)%c.config.PolyBFTConfig.SprintSize == 0
}

// getSystemState builds SystemState instance for the most current block header
func (c *consensusRuntime) getSystemState(header *types.Header) (SystemState, error) {
	provider, err := c.config.blockchain.GetStateProviderForBlock(header)
	if err != nil {
		return nil, err
	}

	return c.config.blockchain.GetSystemState(c.config.PolyBFTConfig, provider), nil
}

func (c *consensusRuntime) IsValidBlock(proposal []byte) bool {
	if err := c.fsm.Validate(proposal); err != nil {
		c.logger.Error("failed to validate proposal", "error", err)

		return false
	}

	return true
}

func (c *consensusRuntime) IsValidSender(msg *proto.Message) bool {
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

func (c *consensusRuntime) IsValidProposalHash(proposal, hash []byte) bool {
	if len(proposal) == 0 {
		c.logger.Error("proposal hash is not valid because proposal is empty")

		return false
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
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

	if sharedData.lastBuiltBlock.Number+1 != view.Height {
		c.logger.Error("unable to build block, due to lack of parent block",
			"last", sharedData.lastBuiltBlock.Number, "num", view.Height)

		return nil
	}

	proposal, err := c.fsm.BuildProposal(view.Round)
	if err != nil {
		c.logger.Info("Unable to create proposal", "blockNumber", view.Height, "error", err)

		return nil
	}

	return proposal
}

// InsertBlock inserts a proposal with the specified committed seals
func (c *consensusRuntime) InsertBlock(proposal []byte, committedSeals []*messages.CommittedSeal) {
	fsm := c.fsm

	block, err := fsm.Insert(proposal, committedSeals)
	if err != nil {
		c.logger.Error("cannot insert proposal", "error", err)

		return
	}

	if c.IsBridgeEnabled() {
		if (fsm.isEndOfEpoch || c.checkpointManager.isCheckpointBlock(block.Header.Number)) &&
			bytes.Equal(c.config.Key.Address().Bytes(), block.Header.Miner) {
			go func(header types.Header, epochNumber uint64) {
				if err := c.checkpointManager.submitCheckpoint(header, fsm.isEndOfEpoch); err != nil {
					c.logger.Warn("failed to submit checkpoint",
						"checkpoint block", header.Number,
						"epoch number", epochNumber,
						"error", err)
				}
			}(*block.Header, fsm.epochNumber)

			c.checkpointManager.latestCheckpointID = block.Number()
		}
	}

	c.OnBlockInserted(block)
}

// ID return ID (address actually) of the current node
func (c *consensusRuntime) ID() []byte {
	return c.config.Key.Address().Bytes()
}

// HasQuorum returns true if quorum is reached for the given blockNumber
func (c *consensusRuntime) HasQuorum(
	blockNumber uint64,
	messages []*proto.Message,
	msgType proto.MessageType,
) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()
	// extract the addresses of all the signers of the messages
	ppIncluded := false
	signers := make(map[types.Address]struct{}, len(messages))

	for _, message := range messages {
		if message.Type == proto.MessageType_PREPREPARE {
			ppIncluded = true
		}

		signers[types.BytesToAddress(message.From)] = struct{}{}
	}

	// check quorum
	switch msgType {
	case proto.MessageType_PREPREPARE:
		return len(messages) >= 1
	case proto.MessageType_PREPARE:
		if ppIncluded {
			return c.fsm.validators.HasQuorum(signers)
		}

		if len(messages) == 0 {
			return false
		}

		propAddress, err := c.fsm.proposerSnapshot.GetLatestProposer(messages[0].View.Round, blockNumber)
		if err != nil {
			c.logger.Warn("HasQuorum has been called but proposer is not set", "error", err)

			return false
		}

		if _, ok := signers[propAddress]; ok {
			c.logger.Warn("HasQuorum failed - proposer is among signers but it is not expected to be")

			return false
		}

		signers[propAddress] = struct{}{} // add proposer manually

		return c.fsm.validators.HasQuorum(signers)
	case proto.MessageType_ROUND_CHANGE, proto.MessageType_COMMIT:
		return c.fsm.validators.HasQuorum(signers)
	default:
		return false
	}
}

// BuildPrePrepareMessage builds a PREPREPARE message based on the passed in proposal
func (c *consensusRuntime) BuildPrePrepareMessage(
	proposal []byte,
	certificate *proto.RoundChangeCertificate,
	view *proto.View,
) *proto.Message {
	if len(proposal) == 0 {
		c.logger.Error("can not build pre-prepare message, since proposal is empty")

		return nil
	}

	block := types.Block{}
	if err := block.UnmarshalRLP(proposal); err != nil {
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
		c.logger.Error("failed to calculate proposal hash for block %d: %w", block.Number(), err)

		return nil
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

	message, err := c.config.Key.SignEcdsaMessage(&msg)
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

	message, err := c.config.Key.SignEcdsaMessage(&msg)
	if err != nil {
		c.logger.Error("Cannot sign message.", "error", err)

		return nil
	}

	return message
}

// BuildCommitMessage builds a COMMIT message based on the passed in proposal
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
		c.logger.Error("Cannot sign message", "Error", err)

		return nil
	}

	return message
}

// BuildRoundChangeMessage builds a ROUND_CHANGE message based on the passed in proposal
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

// createExitTree creates an exit event merkle tree from provided exit events
func createExitTree(exitEvents []*ExitEvent) (*MerkleTree, error) {
	numOfEvents := len(exitEvents)
	data := make([][]byte, numOfEvents)

	for i := 0; i < numOfEvents; i++ {
		b, err := ExitEventABIType.Encode(exitEvents[i])
		if err != nil {
			return nil, err
		}

		data[i] = b
	}

	return NewMerkleTree(data)
}

// getSealersForBlock checks who sealed a given block and updates the counter
func getSealersForBlock(sealersCounter map[types.Address]uint64,
	blockExtra *Extra, validators AccountSet) error {
	signers, err := validators.GetFilteredValidators(blockExtra.Parent.Bitmap)
	if err != nil {
		return err
	}

	for _, a := range signers.GetAddresses() {
		sealersCounter[a]++
	}

	return nil
}
