package polybft

import (
	"fmt"
	"math/big"
	"strconv"
	"time"

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

	// frequency at which checkpoints are sent to the rootchain
	checkpointTimeInterval = 30 * time.Minute
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
	checkpointsOffset uint64
}

// newCheckpointManager creates a new instance of checkpointManager
func newCheckpointManager(sender types.Address, blockTime time.Duration, interactor rootchainInteractor,
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
		checkpointsOffset: uint64(checkpointTimeInterval.Milliseconds() / blockTime.Milliseconds()),
	}
}

// getCurrentCheckpointID queries CheckpointManager smart contract and retrieves current checkpoint id
func (c checkpointManager) getCurrentCheckpointID(epochNumber uint64) (uint64, error) {
	checkpointIDMethodEncoded, err := currentCheckpointIDMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function parameters for epoch=%d: %w",
			epochNumber, err)
	}

	currentCheckpointID, err := c.rootchain.Call(c.sender, helper.CheckpointManagerAddress, checkpointIDMethodEncoded)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointId function on the rootchain for epoch=%d: %w",
			epochNumber, err)
	}

	checkpointID, err := strconv.ParseUint(currentCheckpointID, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint id '%s' to number for epoch=%d: %w",
			currentCheckpointID, epochNumber, err)
	}

	return checkpointID, nil
}

// submitCheckpoint sends a transaction which with checkpoint data to the rootchain
func (c checkpointManager) submitCheckpoint(latestHeader types.Header, epochNumber uint64) error {
	checkpointID, err := c.getCurrentCheckpointID(epochNumber)
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

	// detect any pending (previously failed) checkpoints and send them
	for blockNum := checkpointID + 1; blockNum < latestHeader.Number; blockNum++ {
		header, exists := c.blockchain.GetHeaderByNumber(blockNum)
		if !exists {
			return fmt.Errorf("block %d was not found", blockNum)
		}

		extra, err := GetIbftExtra(header.ExtraData)
		if err != nil {
			return err
		}

		// send checkpoints only for epoch ending blocks (validators delta is non-empty)
		if extra.Validators.IsEmpty() {
			continue
		}

		err = c.submitCheckpointInternal(nonce, txn, *header, *extra)
		if err != nil {
			return err
		}
		nonce++
	}

	extra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return err
	}

	return c.submitCheckpointInternal(nonce, txn, latestHeader, *extra)
}

// submitCheckpointInternal encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) submitCheckpointInternal(nonce uint64, txn *ethgo.Transaction,
	header types.Header, extra Extra) error {
	nextEpochValidators, err := c.consensusBackend.GetValidators(header.Number, nil)
	if err != nil {
		return err
	}

	input, err := c.abiEncodeCheckpointBlock(header, extra, nextEpochValidators)
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
func (c *checkpointManager) abiEncodeCheckpointBlock(header types.Header, extra Extra,
	nextValidators AccountSet) ([]byte, error) {
	params := map[string]interface{}{
		"chainID":             new(big.Int).SetUint64(c.blockchain.GetChainID()),
		"aggregatedSignature": extra.Committed.AggregatedSignature,
		"validatorsBitmap":    extra.Committed.Bitmap,
		"epochNumber":         new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
		"blockNumber":         new(big.Int).SetUint64(header.Number),
		"blockHash":           header.Hash,
		"blockRound":          new(big.Int).SetUint64(extra.Checkpoint.BlockRound),
		"eventRoot":           extra.Checkpoint.EventRoot.Bytes(),
		"nextValidators":      nextValidators.AsGenericMaps(),
	}

	return submitCheckpointMethod.Encode(params)
}

// isCheckpointIntervalElapsed returns indication whether given block is the checkpoint block.
// Returns true at each N blocks, where N is calculated
// as division between predefined checkpoint interval and block time
func (c *checkpointManager) isCheckpointIntervalElapsed(blockNumber uint64) bool {
	return blockNumber%c.checkpointsOffset == 0
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
