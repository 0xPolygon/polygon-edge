package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"

	"github.com/0xPolygon/polygon-edge/bls"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/merkle-tree"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	bolt "go.etcd.io/bbolt"
)

var (
	// currentCheckpointBlockNumMethod is an ABI method object representation for
	// currentCheckpointBlockNumber getter function on CheckpointManager contract
	currentCheckpointBlockNumMethod = contractsapi.CheckpointManager.Abi.Methods["currentCheckpointBlockNumber"]
	// frequency at which checkpoints are sent to the rootchain (in blocks count)
	defaultCheckpointsOffset = uint64(900)
)

type CheckpointManager interface {
	EventSubscriber
	PostBlock(req *PostBlockRequest) error
	BuildEventRoot(epoch uint64) (types.Hash, error)
	GenerateExitProof(exitID uint64) (types.Proof, error)
}

var _ CheckpointManager = (*dummyCheckpointManager)(nil)

type dummyCheckpointManager struct{}

func (d *dummyCheckpointManager) PostBlock(req *PostBlockRequest) error { return nil }
func (d *dummyCheckpointManager) BuildEventRoot(epoch uint64) (types.Hash, error) {
	return types.ZeroHash, nil
}
func (d *dummyCheckpointManager) GenerateExitProof(exitID uint64) (types.Proof, error) {
	return types.Proof{}, nil
}

// EventSubscriber implementation
func (d *dummyCheckpointManager) GetLogFilters() map[types.Address][]types.Hash {
	return make(map[types.Address][]types.Hash)
}
func (d *dummyCheckpointManager) ProcessLog(header *types.Header,
	log *ethgo.Log, dbTx *bolt.Tx) error {
	return nil
}

var _ CheckpointManager = (*checkpointManager)(nil)

// checkpointManager encapsulates logic for checkpoint data submission
type checkpointManager struct {
	// key is the identity of the node submitting a checkpoint
	key ethgo.Key
	// blockchain is abstraction for blockchain
	blockchain blockchainBackend
	// consensusBackend is abstraction for polybft consensus specific functions
	consensusBackend polybftBackend
	// rootChainRelayer abstracts rootchain interaction logic (Call and SendTransaction invocations to the rootchain)
	rootChainRelayer txrelayer.TxRelayer
	// checkpointsOffset represents offset between checkpoint blocks (applicable only for non-epoch ending blocks)
	checkpointsOffset uint64
	// checkpointManagerAddr is address of CheckpointManager smart contract
	checkpointManagerAddr types.Address
	// lastSentBlock represents the last block on which a checkpoint transaction was sent
	lastSentBlock uint64
	// logger instance
	logger hclog.Logger
	// state boltDb instance
	state *State
}

// newCheckpointManager creates a new instance of checkpointManager
func newCheckpointManager(key ethgo.Key, checkpointOffset uint64,
	checkpointManagerSC types.Address, txRelayer txrelayer.TxRelayer,
	blockchain blockchainBackend, backend polybftBackend, logger hclog.Logger,
	state *State) *checkpointManager {
	return &checkpointManager{
		key:                   key,
		blockchain:            blockchain,
		consensusBackend:      backend,
		rootChainRelayer:      txRelayer,
		checkpointsOffset:     checkpointOffset,
		checkpointManagerAddr: checkpointManagerSC,
		logger:                logger,
		state:                 state,
	}
}

// getCurrentCheckpointBlock queries CheckpointManager smart contract and retrieves the current checkpoint block number
func getCurrentCheckpointBlock(relayer txrelayer.TxRelayer, checkpointManagerAddr types.Address) (uint64, error) {
	checkpointBlockNumInput, err := currentCheckpointBlockNumMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointBlockNumber function parameters: %w", err)
	}

	currentCheckpointBlockRaw, err := relayer.Call(ethgo.ZeroAddress, ethgo.Address(checkpointManagerAddr),
		checkpointBlockNumInput)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointBlockNumber function on the rootchain: %w", err)
	}

	currentCheckpointBlock, err := strconv.ParseUint(currentCheckpointBlockRaw, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint block number '%s' to number: %w",
			currentCheckpointBlockRaw, err)
	}

	return currentCheckpointBlock, nil
}

// submitCheckpoint sends a transaction with checkpoint data to the rootchain
func (c *checkpointManager) submitCheckpoint(latestHeader *types.Header, isEndOfEpoch bool) error {
	lastCheckpointBlockNumber, err := getCurrentCheckpointBlock(c.rootChainRelayer, c.checkpointManagerAddr)
	if err != nil {
		return err
	}

	if lastCheckpointBlockNumber > latestHeader.Number {
		// node is out of sync (haven't reached the tip of the chain), so even though it is a proposer,
		// it would checkpoint block that is already checkpointed and transaction would fail anyway
		return nil
	}

	c.logger.Debug("submitCheckpoint invoked...",
		"latest checkpoint block", lastCheckpointBlockNumber,
		"checkpoint block", latestHeader.Number)

	var (
		initialBlockNumber = lastCheckpointBlockNumber + 1
		parentExtra        *Extra
		parentHeader       *types.Header
		currentExtra       *Extra
		found              bool
	)

	if initialBlockNumber < latestHeader.Number {
		parentHeader, found = c.blockchain.GetHeaderByNumber(initialBlockNumber)
		if !found {
			return fmt.Errorf("block %d was not found", initialBlockNumber)
		}

		parentExtra, err = GetIbftExtra(parentHeader.ExtraData)
		if err != nil {
			return err
		}
	}

	// detect any pending (previously failed) checkpoints and send them
	for blockNumber := initialBlockNumber + 1; blockNumber <= latestHeader.Number; blockNumber++ {
		currentHeader, found := c.blockchain.GetHeaderByNumber(blockNumber)
		if !found {
			return fmt.Errorf("block %d was not found", blockNumber)
		}

		currentExtra, err = GetIbftExtra(currentHeader.ExtraData)
		if err != nil {
			return err
		}

		parentEpochNumber := parentExtra.Checkpoint.EpochNumber
		currentEpochNumber := currentExtra.Checkpoint.EpochNumber
		// send pending checkpoints only for epoch ending blocks
		if blockNumber == 1 || parentEpochNumber == currentEpochNumber {
			parentHeader = currentHeader
			parentExtra = currentExtra

			continue
		}

		if err = c.encodeAndSendCheckpoint(parentHeader, parentExtra, true); err != nil {
			return err
		}

		parentHeader = currentHeader
		parentExtra = currentExtra
	}

	// latestHeader extra could be set in the for loop above
	// (in case there were pending checkpoint blocks)
	if currentExtra == nil {
		// we need to send checkpoint for the latest block
		currentExtra, err = GetIbftExtra(latestHeader.ExtraData)
		if err != nil {
			return err
		}
	}

	return c.encodeAndSendCheckpoint(latestHeader, currentExtra, isEndOfEpoch)
}

// encodeAndSendCheckpoint encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) encodeAndSendCheckpoint(header *types.Header, extra *Extra, isEndOfEpoch bool) error {
	c.logger.Debug("send checkpoint txn...", "block number", header.Number)

	checkpointManager := ethgo.Address(c.checkpointManagerAddr)

	nextEpochValidators := validator.AccountSet{}

	if isEndOfEpoch {
		var err error
		nextEpochValidators, err = c.consensusBackend.GetValidators(header.Number, nil)

		if err != nil {
			return err
		}
	}

	input, err := c.abiEncodeCheckpointBlock(header.Number, header.Hash, extra, nextEpochValidators)
	if err != nil {
		return fmt.Errorf("failed to encode checkpoint data to ABI for block %d: %w", header.Number, err)
	}

	txn := &ethgo.Transaction{
		To:    &checkpointManager,
		Input: input,
		Type:  ethgo.TransactionDynamicFee,
	}

	receipt, err := c.rootChainRelayer.SendTransaction(txn, c.key)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("checkpoint submission transaction failed for block %d", header.Number)
	}

	c.logger.Debug("send checkpoint txn success", "block number", header.Number, "gasUsed", receipt.GasUsed)

	return nil
}

// abiEncodeCheckpointBlock encodes checkpoint data into ABI format for a given header
func (c *checkpointManager) abiEncodeCheckpointBlock(blockNumber uint64, blockHash types.Hash, extra *Extra,
	nextValidators validator.AccountSet) ([]byte, error) {
	aggs, err := bls.UnmarshalSignature(extra.Committed.AggregatedSignature)
	if err != nil {
		return nil, err
	}

	encodedAggSigs, err := aggs.ToBigInt()
	if err != nil {
		return nil, err
	}

	submit := &contractsapi.SubmitCheckpointManagerFn{
		CheckpointMetadata: &contractsapi.CheckpointMetadata{
			BlockHash:               blockHash,
			BlockRound:              new(big.Int).SetUint64(extra.Checkpoint.BlockRound),
			CurrentValidatorSetHash: extra.Checkpoint.CurrentValidatorsHash,
		},
		Checkpoint: &contractsapi.Checkpoint{
			Epoch:       new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
			BlockNumber: new(big.Int).SetUint64(blockNumber),
			EventRoot:   extra.Checkpoint.EventRoot,
		},
		Signature:       encodedAggSigs,
		Bitmap:          extra.Committed.Bitmap,
		NewValidatorSet: nextValidators.ToAPIBinding(),
	}

	return submit.EncodeAbi()
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offset by predefined count of blocks
// or if given block is an epoch ending block
func (c *checkpointManager) isCheckpointBlock(blockNumber uint64, isEpochEndingBlock bool) bool {
	return isEpochEndingBlock || blockNumber == c.lastSentBlock+c.checkpointsOffset
}

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It sends a checkpoint if given block is checkpoint block and block proposer is given validator
func (c *checkpointManager) PostBlock(req *PostBlockRequest) error {
	if c.isCheckpointBlock(req.FullBlock.Block.Header.Number, req.IsEpochEndingBlock) &&
		bytes.Equal(c.key.Address().Bytes(), req.FullBlock.Block.Header.Miner) {
		go func(header *types.Header, epochNumber uint64) {
			if err := c.submitCheckpoint(header, req.IsEpochEndingBlock); err != nil {
				c.logger.Warn("failed to submit checkpoint",
					"checkpoint block", header.Number,
					"epoch number", epochNumber,
					"error", err)
			}
		}(req.FullBlock.Block.Header, req.Epoch)

		c.lastSentBlock = req.FullBlock.Block.Number()
	}

	return nil
}

// BuildEventRoot returns an exit event root hash for exit tree of given epoch
func (c *checkpointManager) BuildEventRoot(epoch uint64) (types.Hash, error) {
	exitEvents, err := c.state.CheckpointStore.getExitEventsByEpoch(epoch)
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

// GenerateExitProof generates proof of exit event
func (c *checkpointManager) GenerateExitProof(exitID uint64) (types.Proof, error) {
	c.logger.Debug("Generating proof for exit", "exitID", exitID)

	exitEvent, err := c.state.CheckpointStore.getExitEvent(exitID)
	if err != nil {
		return types.Proof{}, err
	}

	getCheckpointBlockFn := &contractsapi.GetCheckpointBlockCheckpointManagerFn{
		BlockNumber: new(big.Int).SetUint64(exitEvent.BlockNumber),
	}

	input, err := getCheckpointBlockFn.EncodeAbi()
	if err != nil {
		return types.Proof{}, fmt.Errorf("failed to encode get checkpoint block input: %w", err)
	}

	getCheckpointBlockResp, err := c.rootChainRelayer.Call(
		ethgo.ZeroAddress,
		ethgo.Address(c.checkpointManagerAddr),
		input)
	if err != nil {
		return types.Proof{}, fmt.Errorf("failed to retrieve checkpoint block for exit ID %d: %w", exitID, err)
	}

	getCheckpointBlockRespRaw, err := hex.DecodeHex(getCheckpointBlockResp)
	if err != nil {
		return types.Proof{}, fmt.Errorf("failed to decode hex response for exit ID %d: %w", exitID, err)
	}

	getCheckpointBlockGeneric, err := contractsapi.GetCheckpointBlockABIResponse.Decode(getCheckpointBlockRespRaw)
	if err != nil {
		return types.Proof{}, fmt.Errorf("failed to decode checkpoint block response for exit ID %d: %w", exitID, err)
	}

	checkpointBlockMap, ok := getCheckpointBlockGeneric.(map[string]interface{})
	if !ok {
		return types.Proof{}, fmt.Errorf("failed to convert for checkpoint block response exit ID %d", exitID)
	}

	isFoundGeneric, ok := checkpointBlockMap["isFound"]
	if !ok {
		return types.Proof{}, fmt.Errorf("invalid response for exit ID %d", exitID)
	}

	isCheckpointFound, ok := isFoundGeneric.(bool)
	if !ok || !isCheckpointFound {
		return types.Proof{}, fmt.Errorf("checkpoint block not found for exit ID %d", exitID)
	}

	checkpointBlockGeneric, ok := checkpointBlockMap["checkpointBlock"]
	if !ok {
		return types.Proof{}, fmt.Errorf("checkpoint block not found for exit ID %d", exitID)
	}

	checkpointBlock, ok := checkpointBlockGeneric.(*big.Int)
	if !ok {
		return types.Proof{}, fmt.Errorf("checkpoint block not found for exit ID %d", exitID)
	}

	exitEventEncoded, err := exitEvent.L2StateSyncedEvent.Encode()
	if err != nil {
		return types.Proof{}, err
	}

	exitEvents, err := c.state.CheckpointStore.getExitEventsForProof(exitEvent.EpochNumber, checkpointBlock.Uint64())
	if err != nil {
		return types.Proof{}, err
	}

	tree, err := createExitTree(exitEvents)
	if err != nil {
		return types.Proof{}, err
	}

	leafIndex, err := tree.LeafIndex(exitEventEncoded)
	if err != nil {
		return types.Proof{}, err
	}

	proof, err := tree.GenerateProof(exitEventEncoded)
	if err != nil {
		return types.Proof{}, err
	}

	c.logger.Debug("Generated proof for exit", "exitID", exitID, "leafIndex", leafIndex, "proofLen", len(proof))

	exitEventHex := hex.EncodeToString(exitEventEncoded)

	return types.Proof{
		Data: proof,
		Metadata: map[string]interface{}{
			"LeafIndex":       leafIndex,
			"ExitEvent":       exitEventHex,
			"CheckpointBlock": checkpointBlock,
		},
	}, nil
}

// EventSubscriber implementation

// GetLogFilters returns a map of log filters for getting desired events,
// where the key is the address of contract that emits desired events,
// and the value is a slice of signatures of events we want to get.
// This function is the implementation of EventSubscriber interface
func (c *checkpointManager) GetLogFilters() map[types.Address][]types.Hash {
	var l2StateSyncedEvent contractsapi.L2StateSyncedEvent

	return map[types.Address][]types.Hash{
		contracts.L2StateSenderContract: {types.Hash(l2StateSyncedEvent.Sig())},
	}
}

// ProcessLog is the implementation of EventSubscriber interface,
// used to handle a log defined in GetLogFilters, provided by event provider
func (c *checkpointManager) ProcessLog(header *types.Header, log *ethgo.Log, dbTx *bolt.Tx) error {
	exitEvent, doesMatch, err := parseExitEvent(header, log)
	if err != nil {
		return err
	}

	if !doesMatch {
		return nil
	}

	return c.state.CheckpointStore.insertExitEvent(exitEvent, dbTx)
}

// createExitTree creates an exit event merkle tree from provided exit events
func createExitTree(exitEvents []*ExitEvent) (*merkle.MerkleTree, error) {
	numOfEvents := len(exitEvents)
	data := make([][]byte, numOfEvents)

	for i := 0; i < numOfEvents; i++ {
		b, err := exitEvents[i].L2StateSyncedEvent.Encode()
		if err != nil {
			return nil, err
		}

		data[i] = b
	}

	return merkle.NewMerkleTree(data)
}

// parseExitEvent parses exit event from provided log
func parseExitEvent(h *types.Header, l *ethgo.Log) (*ExitEvent, bool, error) {
	extra, err := GetIbftExtra(h.ExtraData)
	if err != nil {
		return nil, false,
			fmt.Errorf("could not get header extra on exit event parsing. Error: %w", err)
	}

	epoch := extra.Checkpoint.EpochNumber
	block := h.Number

	if extra.Validators != nil {
		// exit events that happened in epoch ending blocks,
		// should be added to the tree of the next epoch
		epoch++
		block++
	}

	event, err := decodeExitEvent(l, epoch, block)
	if err != nil {
		return nil, false, err
	}

	return event, true, nil
}
