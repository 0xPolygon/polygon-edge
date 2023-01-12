package polybft

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"

	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	metrics "github.com/armon/go-metrics"
	hclog "github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	// currentCheckpointBlockNumMethod is an ABI method object representation for
	// currentCheckpointBlockNumber getter function on CheckpointManager contract
	currentCheckpointBlockNumMethod, _ = abi.NewMethod("function currentCheckpointBlockNumber() returns (uint256)")

	// submitCheckpointMethod is an ABI method object representation for
	// submit checkpoint function on CheckpointManager contract
	submitCheckpointMethod, _ = abi.NewMethod("function submit(" +
		"uint256 chainId," +
		"tuple(bytes32 blockHash, uint256 blockRound, bytes32 currentValidatorSetHash) checkpointMetadata," +
		"tuple(uint256 epochNumber, uint256 blockNumber, bytes32 eventRoot) checkpoint," +
		"uint256[2] signature," +
		"tuple(address _address, uint256[4] blsKey, uint256 votingPower)[] newValidatorSet," +
		"bytes bitmap)")

	// frequency at which checkpoints are sent to the rootchain (in blocks count)
	defaultCheckpointsOffset = uint64(900)
)

// checkpointManager encapsulates logic for checkpoint data submission
type checkpointManager struct {
	// key is the identity of the node submitting a checkpoint
	key ethgo.Key
	// blockchain is abstraction for blockchain
	blockchain blockchainBackend
	// consensusBackend is abstraction for polybft consensus specific functions
	consensusBackend polybftBackend
	// txRelayer abstracts rootchain interaction logic (Call and SendTransaction invocations to the rootchain)
	txRelayer txrelayer.TxRelayer
	// checkpointsOffset represents offset between checkpoint blocks (applicable only for non-epoch ending blocks)
	checkpointsOffset uint64
	// checkpointManagerAddr is address of CheckpointManager smart contract
	checkpointManagerAddr types.Address
	// latestCheckpointID represents last checkpoint block number
	latestCheckpointID uint64
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
		txRelayer:             txRelayer,
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

	latestCheckpointBlockRaw, err := c.txRelayer.Call(
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
func (c *checkpointManager) submitCheckpoint(latestHeader types.Header, isEndOfEpoch bool) error {
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
	)

	if initialBlockNumber < latestHeader.Number {
		found := false
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

		if err = c.encodeAndSendCheckpoint(txn, *parentHeader, *parentExtra, true); err != nil {
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

	return c.encodeAndSendCheckpoint(txn, latestHeader, *currentExtra, isEndOfEpoch)
}

// encodeAndSendCheckpoint encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) encodeAndSendCheckpoint(txn *ethgo.Transaction,
	header types.Header, extra Extra, isEndOfEpoch bool) error {
	c.logger.Debug("send checkpoint txn...", "block number", header.Number)

	nextEpochValidators := AccountSet{}

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

	receipt, err := c.txRelayer.SendTransaction(txn, c.key)
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
func (c *checkpointManager) abiEncodeCheckpointBlock(blockNumber uint64, blockHash types.Hash, extra Extra,
	nextValidators AccountSet) ([]byte, error) {
	aggs, err := bls.UnmarshalSignature(extra.Committed.AggregatedSignature)
	if err != nil {
		return nil, err
	}

	encodedAggSigs, err := aggs.ToBigInt()
	if err != nil {
		return nil, err
	}

	params := map[string]interface{}{
		"chainId": new(big.Int).SetUint64(c.blockchain.GetChainID()),
		"checkpointMetadata": map[string]interface{}{
			"blockHash":               blockHash,
			"blockRound":              new(big.Int).SetUint64(extra.Checkpoint.BlockRound),
			"currentValidatorSetHash": extra.Checkpoint.CurrentValidatorsHash,
		},
		"checkpoint": map[string]interface{}{
			"epochNumber": new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
			"blockNumber": new(big.Int).SetUint64(blockNumber),
			"eventRoot":   extra.Checkpoint.EventRoot,
		},
		"signature":       encodedAggSigs,
		"newValidatorSet": nextValidators.AsGenericMaps(),
		"bitmap":          extra.Committed.Bitmap,
	}

	return submitCheckpointMethod.Encode(params)
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offset by predefined count of blocks
func (c *checkpointManager) isCheckpointBlock(blockNumber uint64) bool {
	return blockNumber == c.latestCheckpointID+c.checkpointsOffset
}

// PostBlock is called on every insert of finalized block (either from consensus or syncer)
// It will read any exit event that happened in block and insert it to state boltDb
func (c *checkpointManager) PostBlock(req *PostBlockRequest) error {
	epoch := req.Epoch
	if req.IsEpochEndingBlock {
		// exit events that happened in epoch ending blocks,
		// should be added to the tree of the next epoch
		epoch++
	}

	// commit exit events only when we finalize a block
	events, err := getExitEventsFromReceipts(epoch, req.FullBlock.Block.Number(), req.FullBlock.Receipts)
	if err != nil {
		return err
	}

	if len(events) > 0 {
		if err := c.state.insertExitEvents(events); err != nil {
			return err
		}
	}

	return nil
}

// getExitEventsFromReceipts parses logs from receipts to find exit events
func getExitEventsFromReceipts(epoch, block uint64, receipts []*types.Receipt) ([]*ExitEvent, error) {
	events := make([]*ExitEvent, 0)

	for i := 0; i < len(receipts); i++ {
		if len(receipts[i].Logs) == 0 {
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
