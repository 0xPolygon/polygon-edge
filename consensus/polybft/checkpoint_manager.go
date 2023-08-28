package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"sort"
	"strconv"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/validator"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/merkle-tree"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	metrics "github.com/armon/go-metrics"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
)

var (
	// currentCheckpointBlockNumMethod is an ABI method object representation for
	// currentCheckpointBlockNumber getter function on CheckpointManager contract
	currentCheckpointBlockNumMethod, _ = contractsapi.CheckpointManager.Abi.Methods["currentCheckpointBlockNumber"]
	// frequency at which checkpoints are sent to the rootchain (in blocks count)
	defaultCheckpointsOffset = uint64(900)
)

type CheckpointManager interface {
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

// getLatestCheckpointBlock queries CheckpointManager smart contract and retrieves latest checkpoint block number
func (c *checkpointManager) getLatestCheckpointBlock() (uint64, error) {
	checkpointBlockNumMethodEncoded, err := currentCheckpointBlockNumMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function parameters: %w", err)
	}

	latestCheckpointBlockRaw, err := c.rootChainRelayer.Call(
		c.key.Address(),
		ethgo.Address(c.checkpointManagerAddr),
		checkpointBlockNumMethodEncoded)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointId function on the rootchain: %w", err)
	}

	latestCheckpointBlockNum, err := strconv.ParseUint(latestCheckpointBlockRaw, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint id '%s' to number: %w",
			latestCheckpointBlockRaw, err)
	}

	return latestCheckpointBlockNum, nil
}

// submitCheckpoint sends a transaction with checkpoint data to the rootchain
func (c *checkpointManager) submitCheckpoint(latestHeader *types.Header, isEndOfEpoch bool) error {
	lastCheckpointBlockNumber, err := c.getLatestCheckpointBlock()
	if err != nil {
		return err
	}

	c.logger.Debug("submitCheckpoint invoked...",
		"latest checkpoint block", lastCheckpointBlockNumber,
		"checkpoint block", latestHeader.Number)

	checkpointManagerAddr := ethgo.Address(c.checkpointManagerAddr)
	txn := &ethgo.Transaction{
		To:   &checkpointManagerAddr,
		From: c.key.Address(),
	}
	initialBlockNumber := lastCheckpointBlockNumber + 1

	var (
		parentExtra  *Extra
		parentHeader *types.Header
		currentExtra *Extra
		found        bool
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

		if err = c.encodeAndSendCheckpoint(txn, parentHeader, parentExtra, true); err != nil {
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

	return c.encodeAndSendCheckpoint(txn, latestHeader, currentExtra, isEndOfEpoch)
}

// encodeAndSendCheckpoint encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) encodeAndSendCheckpoint(txn *ethgo.Transaction,
	header *types.Header, extra *Extra, isEndOfEpoch bool) error {
	c.logger.Debug("send checkpoint txn...", "block number", header.Number)

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

	txn.Input = input

	receipt, err := c.rootChainRelayer.SendTransaction(txn, c.key)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("checkpoint submission transaction failed for block %d", header.Number)
	}

	// update checkpoint block number metrics
	metrics.SetGauge([]string{"bridge", "checkpoint_block_number"}, float32(header.Number))
	c.logger.Debug("send checkpoint txn success", "block number", header.Number)

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
// It will read any exit event that happened in block and insert it to state boltDb
func (c *checkpointManager) PostBlock(req *PostBlockRequest) error {
	var (
		epoch = req.Epoch
		block = req.FullBlock.Block.Number()
	)

	if req.IsEpochEndingBlock {
		// exit events that happened in epoch ending blocks,
		// should be added to the tree of the next epoch
		epoch++
		block++
	}

	// commit exit events only when we finalize a block
	events, err := getExitEventsFromReceipts(epoch, block, req.FullBlock.Receipts)
	if err != nil {
		return err
	}

	if len(events) > 0 {
		c.logger.Debug("Gotten exit events from logs on block",
			"eventsNum", len(events), "block", req.FullBlock.Block.Number())
	}

	if err := c.state.CheckpointStore.insertExitEvents(events); err != nil {
		return err
	}

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

	var exitEventAPI contractsapi.L2StateSyncedEvent

	e, err := exitEventAPI.Encode(exitEvent)
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

	leafIndex, err := tree.LeafIndex(e)
	if err != nil {
		return types.Proof{}, err
	}

	proof, err := tree.GenerateProof(e)
	if err != nil {
		return types.Proof{}, err
	}

	c.logger.Debug("Generated proof for exit", "exitID", exitID, "leafIndex", leafIndex, "proofLen", len(proof))

	return types.Proof{
		Data: proof,
		Metadata: map[string]interface{}{
			"LeafIndex":       leafIndex,
			"ExitEvent":       exitEvent,
			"CheckpointBlock": checkpointBlock,
		},
	}, nil
}

// getExitEventsFromReceipts parses logs from receipts to find exit events
func getExitEventsFromReceipts(epoch, block uint64, receipts []*types.Receipt) ([]*ExitEvent, error) {
	events := make([]*ExitEvent, 0)

	for i := 0; i < len(receipts); i++ {
		if receipts[i].Status == nil || *receipts[i].Status != types.ReceiptSuccess {
			continue
		}

		for _, log := range receipts[i].Logs {
			if log.Address != contracts.L2StateSenderContract {
				continue
			}

			event, err := decodeExitEvent(convertLog(log), epoch, block)
			if err != nil {
				return nil, err
			}

			if event == nil {
				// valid case, not an exit event
				continue
			}

			events = append(events, event)
		}
	}

	// enforce sequential order
	sort.Slice(events, func(i, j int) bool {
		return events[i].ID < events[j].ID
	})

	return events, nil
}

// createExitTree creates an exit event merkle tree from provided exit events
func createExitTree(exitEvents []*ExitEvent) (*merkle.MerkleTree, error) {
	numOfEvents := len(exitEvents)
	data := make([][]byte, numOfEvents)

	var exitEventAPI contractsapi.L2StateSyncedEvent
	for i := 0; i < numOfEvents; i++ {
		b, err := exitEventAPI.Encode(exitEvents[i])
		if err != nil {
			return nil, err
		}

		data[i] = b
	}

	return merkle.NewMerkleTree(data)
}
