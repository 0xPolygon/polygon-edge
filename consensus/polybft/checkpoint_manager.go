package polybft

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
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
	// signer is the identity of the node submitting a checkpoint
	signer ethgo.Key
	// signerAddress is the address of the node submitting a checkpoint
	signerAddress types.Address
	// blockchain is abstraction for blockchain
	blockchain blockchainBackend
	// consensusBackend is abstraction for polybft consensus specific functions
	consensusBackend polybftBackend
	// txRelayer abstracts rootchain interaction logic (Call and SendTransaction invocations to the rootchain)
	txRelayer txrelayer.TxRelayer
	// checkpointsOffset represents offset between checkpoint blocks (applicable only for non-epoch ending blocks)
	checkpointsOffset uint64
	// latestCheckpointID represents last checkpointed block number
	latestCheckpointID uint64
	// logger instance
	logger hclog.Logger
}

// newCheckpointManager creates a new instance of checkpointManager
func newCheckpointManager(signer ethgo.Key, checkpointOffset uint64, txRelayer txrelayer.TxRelayer,
	blockchain blockchainBackend, backend polybftBackend, logger hclog.Logger) *checkpointManager {
	return &checkpointManager{
		signer:            signer,
		signerAddress:     types.Address(signer.Address()),
		blockchain:        blockchain,
		consensusBackend:  backend,
		txRelayer:         txRelayer,
		checkpointsOffset: checkpointOffset,
		logger:            logger,
	}
}

// getLatestCheckpointBlock queries CheckpointManager smart contract and retrieves latest checkpoint block number
func (c *checkpointManager) getLatestCheckpointBlock() (uint64, error) {
	checkpointBlockNumMethodEncoded, err := currentCheckpointBlockNumMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function parameters: %w", err)
	}

	latestCheckpointBlockRaw, err := c.txRelayer.Call(
		ethgo.Address(c.signerAddress),
		ethgo.Address(helper.CheckpointManagerAddress),
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

	checkpointManagerAddr := ethgo.Address(helper.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To:   &checkpointManagerAddr,
		From: ethgo.Address(c.signerAddress),
	}
	initialBlockNumber := lastCheckpointBlockNumber + 1

	var (
		parentExtra  *Extra
		parentHeader *types.Header
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
	for blockNumber := initialBlockNumber + 1; blockNumber < latestHeader.Number; blockNumber++ {
		currentHeader, found := c.blockchain.GetHeaderByNumber(blockNumber)
		if !found {
			return fmt.Errorf("block %d was not found", blockNumber)
		}

		currentExtra, err := GetIbftExtra(currentHeader.ExtraData)
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

	// we need to send checkpoint for the latest block
	extra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return err
	}

	return c.encodeAndSendCheckpoint(txn, latestHeader, *extra, isEndOfEpoch)
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

	receipt, err := c.txRelayer.SendTransaction(txn, c.signer)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("checkpoint submission transaction failed for block %d", header.Number)
	}

	c.logger.Debug("send checkpoint txn success", "block number", header.Number)

	return nil
}

// abiEncodeCheckpointBlock encodes checkpoint data into ABI format for a given header
func (c *checkpointManager) abiEncodeCheckpointBlock(headerNumber uint64, headerHash types.Hash, extra Extra,
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
			"blockHash":               headerHash,
			"blockRound":              new(big.Int).SetUint64(extra.Checkpoint.BlockRound),
			"currentValidatorSetHash": extra.Checkpoint.CurrentValidatorsHash,
		},
		"checkpoint": map[string]interface{}{
			"epochNumber": new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
			"blockNumber": new(big.Int).SetUint64(headerNumber),
			"eventRoot":   extra.Checkpoint.EventRoot,
		},
		"signature":       encodedAggSigs,
		"newValidatorSet": nextValidators.AsGenericMaps(),
		"bitmap":          extra.Committed.Bitmap,
	}

	return submitCheckpointMethod.Encode(params)
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offseted by predefined count of blocks
func (c *checkpointManager) isCheckpointBlock(blockNumber uint64) bool {
	return blockNumber == c.latestCheckpointID+c.checkpointsOffset
}
