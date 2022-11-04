package polybft

import (
	"fmt"
	"math/big"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	// currentCheckpointIDMethod is an ABI method object representation for
	// currentCheckpointId getter function on CheckpointManager contract
	currentCheckpointIDMethod, _ = abi.NewMethod("function currentCheckpointId() returns (uint256)")

	// submitCheckpointMethod is an ABI method object representation for
	// submit checkpoint function on CheckpointManager contract
	submitCheckpointMethod, _ = abi.NewMethod("function submitCheckpoint(" +
		"uint256 chainID, bytes aggregatedSignature, bytes validatorsBitmap, " +
		"uint256 epochNumber, uint256 blockNumber, bytes32 blockHash, uint256 blockRound," +
		"bytes32 eventRoot, tuple(address _address, uint256[4] blsKey)[] nextValidators" + ")")

	// frequency at which checkpoints are sent to the rootchain (in blocks count)
	defaultCheckpointsOffset = uint64(900)
)

// checkpointManager encapsulates logic for checkpoint data submission
type checkpointManager struct {
	// sender address
	sender types.Address
	// blockchain is abstraction for blockchain
	blockchain blockchainBackend
	// consensusBackend is abstraction for polybft consensus specific functions
	consensusBackend polybftBackend
	// rootchain represents abstraction for rootchain interaction
	rootchain rootchainInteractor
	// checkpointsOffset represents offset between checkpoint blocks (applicable only for non-epoch ending blocks)
	checkpointsOffset    uint64
	lastCheckpointNumber uint64
}

// newCheckpointManager creates a new instance of checkpointManager
func newCheckpointManager(sender types.Address, checkpointOffset uint64, interactor rootchainInteractor,
	blockchain blockchainBackend, backend polybftBackend) *checkpointManager {
	r := interactor
	if interactor == nil {
		r = &defaultRootchainInteractor{}
	}

	return &checkpointManager{
		sender:            sender,
		blockchain:        blockchain,
		consensusBackend:  backend,
		rootchain:         r,
		checkpointsOffset: checkpointOffset,
	}
}

// getCurrentCheckpointID queries CheckpointManager smart contract and retrieves current checkpoint id
func (c checkpointManager) getCurrentCheckpointID() (uint64, error) {
	checkpointIDMethodEncoded, err := currentCheckpointIDMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function parameters: %w", err)
	}

	currentCheckpointID, err := c.rootchain.Call(c.sender, helper.CheckpointManagerAddress, checkpointIDMethodEncoded)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointId function on the rootchain: %w", err)
	}

	checkpointID, err := strconv.ParseUint(currentCheckpointID, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint id '%s' to number: %w",
			currentCheckpointID, err)
	}

	return checkpointID, nil
}

// submitCheckpoint sends a transaction which with checkpoint data to the rootchain
func (c checkpointManager) submitCheckpoint(latestHeader types.Header, isEndOfEpoch bool) error {
	currentCheckpointID, err := c.getCurrentCheckpointID()
	if err != nil {
		return err
	}

	nonce, err := c.rootchain.GetPendingNonce(c.sender)
	if err != nil {
		return err
	}

	checkpointManagerAddr := ethgo.Address(helper.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To: &checkpointManagerAddr,
	}

	var found bool

	var currentHeader, nextHeader *types.Header

	initialBlockNumber := currentCheckpointID + 1

	currentHeader, found = c.blockchain.GetHeaderByNumber(initialBlockNumber)
	if !found {
		return fmt.Errorf("block %d was not found", initialBlockNumber)
	}

	nextHeader, found = c.blockchain.GetHeaderByNumber(initialBlockNumber + 1)
	if !found {
		return fmt.Errorf("block %d was not found", initialBlockNumber+1)
	}

	// detect any pending end-of-epoch (previously failed) checkpoints and send them
	for nextHeader.Number < latestHeader.Number {
		currentExtra, err := GetIbftExtra(currentHeader.ExtraData)
		if err != nil {
			return err
		}

		nextExtra, err := GetIbftExtra(currentHeader.ExtraData)
		if err != nil {
			return err
		}

		//pass only endOfEpoch blocks
		if currentExtra.Checkpoint.EpochNumber == nextExtra.Checkpoint.EpochNumber {
			continue
		}

		currentCheckpointID = currentHeader.Number

		err = c.submitCheckpointInternal(nonce, txn, *currentHeader, *currentExtra, true)
		if err != nil {
			return err
		}

		currentHeader = nextHeader

		nextHeader, found = c.blockchain.GetHeaderByNumber(currentHeader.Number + 1)
		if !found {
			return fmt.Errorf("block %d was not found", initialBlockNumber+1)
		}
		nonce++
	}

	//we need to send checkpoint for the latest block
	extra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return err
	}

	return c.submitCheckpointInternal(nonce, txn, latestHeader, *extra, isEndOfEpoch)
}

// submitCheckpointInternal encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) submitCheckpointInternal(nonce uint64, txn *ethgo.Transaction,
	header types.Header, extra Extra, isEndOfEpoch bool) error {
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

	receipt, err := c.rootchain.SendTransaction(nonce, txn)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("transaction execution failed for block %d", header.Number)
	}

	return nil
}

// abiEncodeCheckpointBlock encodes checkpoint data into ABI format for a given header
func (c *checkpointManager) abiEncodeCheckpointBlock(headerNumber uint64, headerHash types.Hash, extra Extra,
	nextValidators AccountSet) ([]byte, error) {
	params := map[string]interface{}{
		"chainID":             new(big.Int).SetUint64(c.blockchain.GetChainID()),
		"aggregatedSignature": extra.Committed.AggregatedSignature,
		"validatorsBitmap":    extra.Committed.Bitmap,
		"epochNumber":         new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
		"blockNumber":         new(big.Int).SetUint64(headerNumber),
		"blockHash":           headerHash,
		"blockRound":          new(big.Int).SetUint64(extra.Checkpoint.BlockRound),
		"eventRoot":           extra.Checkpoint.EventRoot.Bytes(),
		"nextValidators":      nextValidators.AsGenericMaps(),
	}

	return submitCheckpointMethod.Encode(params)
}

// setCheckpointsOffset sets new checkpointsOffset value
func (c *checkpointManager) setCheckpointsOffset(checkpointsOffset uint64) {
	c.checkpointsOffset = checkpointsOffset
}

// isCheckpointBlock returns true for epoch ending blocks and
// blocks in the middle of the epoch which are offseted by predefined count of blocks
func (c *checkpointManager) isCheckpointBlock(blockNumber uint64) bool {
	return blockNumber == c.lastCheckpointNumber+c.checkpointsOffset
}

var _ rootchainInteractor = (*defaultRootchainInteractor)(nil)

type rootchainInteractor interface {
	Call(from types.Address, to types.Address, input []byte) (string, error)
	SendTransaction(nonce uint64, transaction *ethgo.Transaction) (*ethgo.Receipt, error)
	GetPendingNonce(address types.Address) (uint64, error)
}

type defaultRootchainInteractor struct {
}

func (d *defaultRootchainInteractor) Call(from types.Address, to types.Address, input []byte) (string, error) {
	return helper.Call(ethgo.Address(from), ethgo.Address(to), input)
}

func (d *defaultRootchainInteractor) SendTransaction(nonce uint64,
	transaction *ethgo.Transaction) (*ethgo.Receipt, error) {
	return helper.SendTxn(nonce, transaction)
}

func (d *defaultRootchainInteractor) GetPendingNonce(address types.Address) (uint64, error) {
	return helper.GetPendingNonce(address)
}
