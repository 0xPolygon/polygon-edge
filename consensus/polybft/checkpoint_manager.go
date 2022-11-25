package polybft

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"

	"github.com/0xPolygon/polygon-edge/command/rootchain/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/bitmap"
	bls "github.com/0xPolygon/polygon-edge/consensus/polybft/signer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var (
	// currentCheckpointIDMethod is an ABI method object representation for
	// currentCheckpointId getter function on CheckpointManager contract
	currentCheckpointIDMethod, _ = abi.NewMethod("function latestCheckpointBlockNumber() returns (uint256)")

	// submitCheckpointMethod is an ABI method object representation for
	// submit checkpoint function on CheckpointManager contract
	submitCheckpointMethod, _ = abi.NewMethod("function submit(" +
		"uint256 chainId," +
		"tuple(bytes32 blockHash, uint256 blockRound, bytes32 currentValidatorSetHash) checkpointMetadata," +
		"tuple(uint256 epoch, uint256 blockNumber, bytes32 eventRoot) checkpoint," +
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
	// rootchain represents abstraction for rootchain interaction
	rootchain rootchainInteractor
	// checkpointsOffset represents offset between checkpoint blocks (applicable only for non-epoch ending blocks)
	checkpointsOffset uint64
	// latestCheckpointID represents last checkpointed block number
	latestCheckpointID uint64

	logger hclog.Logger
}

// newCheckpointManager creates a new instance of checkpointManager
func newCheckpointManager(signer ethgo.Key, checkpointOffset uint64, interactor rootchainInteractor,
	blockchain blockchainBackend, backend polybftBackend) *checkpointManager {
	r := interactor
	if interactor == nil {
		r = &defaultRootchainInteractor{}
	}

	return &checkpointManager{
		signer:            signer,
		signerAddress:     types.Address(signer.Address()),
		blockchain:        blockchain,
		consensusBackend:  backend,
		rootchain:         r,
		checkpointsOffset: checkpointOffset,
	}
}

// getLatestCheckpointBlock queries CheckpointManager smart contract and retrieves latest checkpoint block number
func (c checkpointManager) getLatestCheckpointBlock() (uint64, error) {
	checkpointIDMethodEncoded, err := currentCheckpointIDMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function parameters: %w", err)
	}

	latestCheckpointBlockRaw, err := c.rootchain.Call(
		c.signerAddress,
		helper.CheckpointManagerAddress,
		checkpointIDMethodEncoded)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointId function on the rootchain: %w", err)
	}

	latestCheckpointBlockNum, err := strconv.ParseUint(latestCheckpointBlockRaw, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint id '%s' to number: %w",
			latestCheckpointBlockRaw, err)
	}

	c.logger.Info("[checkpoint] latest checkpoint block", "number", latestCheckpointBlockNum)

	return latestCheckpointBlockNum, nil
}

// submitCheckpoint sends a transaction with checkpoint data to the rootchain
func (c checkpointManager) submitCheckpoint(latestHeader types.Header, isEndOfEpoch bool) error {
	c.logger.Info("[checkpoint] Submitting checkpoint...", "block", latestHeader.Number)

	lastCheckpointBlockNumber, err := c.getLatestCheckpointBlock()
	if err != nil {
		return err
	}

	nonce, err := c.rootchain.GetPendingNonce(c.signerAddress)
	if err != nil {
		return err
	}

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
		parentHeader, found = c.blockchain.GetHeaderByNumber(lastCheckpointBlockNumber)

		if !found {
			return fmt.Errorf("block %d was not found", lastCheckpointBlockNumber)
		}

		parentExtra, err = GetIbftExtra(parentHeader.ExtraData)
		if err != nil {
			return err
		}
	}

	// detect any pending (previously failed) checkpoints and send them
	for blockNumber := initialBlockNumber; blockNumber < latestHeader.Number; blockNumber++ {
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
		parentHeader = currentHeader
		parentExtra = currentExtra

		// send pending checkpoints only for epoch ending blocks
		if blockNumber == 1 || parentEpochNumber == currentEpochNumber {
			continue
		}

		if err = c.encodeAndSendCheckpoint(nonce, txn, *parentHeader, *parentExtra, true); err != nil {
			return err
		}
		nonce++
	}

	// we need to send checkpoint for the latest block
	extra, err := GetIbftExtra(latestHeader.ExtraData)
	if err != nil {
		return err
	}

	return c.encodeAndSendCheckpoint(nonce, txn, latestHeader, *extra, isEndOfEpoch)
}

// encodeAndSendCheckpoint encodes checkpoint data for the given block and
// sends a transaction to the CheckpointManager rootchain contract
func (c *checkpointManager) encodeAndSendCheckpoint(nonce uint64, txn *ethgo.Transaction,
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

	receipt, err := c.rootchain.SendTransaction(nonce, txn, c.signer)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("transaction execution failed for block %d", header.Number)
	}

	c.logger.Info("[checkpoint] Submitting checkpoint done successfully.",
		"block", header.Number, "epoch", extra.Checkpoint.EpochNumber)

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
			"epoch":       new(big.Int).SetUint64(extra.Checkpoint.EpochNumber),
			"blockNumber": new(big.Int).SetUint64(headerNumber),
			"eventRoot":   extra.Checkpoint.EventRoot,
		},
		"signature":       encodedAggSigs,
		"newValidatorSet": nextValidators.AsGenericMaps(),
		"bitmap":          extra.Committed.Bitmap,
	}

	c.logger.Info("[checkpoint]", "aggregated signature",
		fmt.Sprintf("[0]=%s, [1]=%s", encodedAggSigs[0], encodedAggSigs[1]))

	currentValidators, err := c.consensusBackend.GetValidators(headerNumber-1, nil)
	if err != nil {
		return nil, err
	}

	filteredValidators, err := currentValidators.GetFilteredValidators(extra.Committed.Bitmap)
	if err != nil {
		return nil, err
	}

	bmp := bitmap.Bitmap(extra.Committed.Bitmap)

	var buffer bytes.Buffer

	for i := uint64(0); i < bmp.Len(); i++ {
		if bmp.IsSet(i) {
			buffer.WriteString("1")
		} else {
			buffer.WriteString("0")
		}
	}

	c.logger.Info("[checkpoint] filtered bls keys", "bitmap", buffer.String())

	for i, v := range filteredValidators {
		bigIntsBlsKey := v.BlsKey.ToBigInt()
		c.logger.Info(fmt.Sprintf("[checkpoint] %d. address=%s", i+1, v.Address))
		c.logger.Info(fmt.Sprintf("[checkpoint] %d. bls key [0]=%s, [1]=%s, [2]=%s, [3]=%s",
			i+1, bigIntsBlsKey[0], bigIntsBlsKey[1], bigIntsBlsKey[2], bigIntsBlsKey[3]))
	}

	c.logger.Info("[checkpoint] all bls keys")

	for i, v := range currentValidators {
		bigIntsBlsKey := v.BlsKey.ToBigInt()
		c.logger.Info(fmt.Sprintf("[checkpoint] %d. bls key [0]=%s, [1]=%s, [2]=%s, [3]=%s",
			i+1, bigIntsBlsKey[0], bigIntsBlsKey[1], bigIntsBlsKey[2], bigIntsBlsKey[3]))
	}

	hash, err := extra.Checkpoint.Hash(c.blockchain.GetChainID(), headerNumber, headerHash)
	if err != nil {
		return nil, err
	}

	aggregatedKey := bls.AggregatePublicKeys(filteredValidators.GetBlsKeys()).ToBigInt()

	c.logger.Info(fmt.Sprintf("[checkpoint] Aggregated key [0]=%s, [1]=%s, [2]=%s, [3]=%s",
		aggregatedKey[0], aggregatedKey[1], aggregatedKey[2], aggregatedKey[3]))

	g1Point, err := bls.G1HashToPoint(hash.Bytes())
	if err != nil {
		return nil, err
	}

	g1BigInts, err := bls.G1ToBigInt(g1Point)
	if err != nil {
		return nil, err
	}

	c.logger.Info("[checkpoing]", "G1 point", fmt.Sprintf("[0]=%s [1]=%s", g1BigInts[0], g1BigInts[1]))

	if aggs.VerifyAggregated(filteredValidators.GetBlsKeys(), hash.Bytes()) {
		c.logger.Info("[checkpoint] VERIFY SIGS SUCCESS")
	} else {
		c.logger.Error("[checkpoint] VERIFY SIGS FAIL")
	}

	return submitCheckpointMethod.Encode(params)
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offseted by predefined count of blocks
func (c *checkpointManager) isCheckpointBlock(blockNumber uint64) bool {
	return blockNumber == c.latestCheckpointID+c.checkpointsOffset
}

var _ rootchainInteractor = (*defaultRootchainInteractor)(nil)

type rootchainInteractor interface {
	Call(from types.Address, to types.Address, input []byte) (string, error)
	SendTransaction(nonce uint64, transaction *ethgo.Transaction, signer ethgo.Key) (*ethgo.Receipt, error)
	GetPendingNonce(address types.Address) (uint64, error)
}

type defaultRootchainInteractor struct {
}

func (d *defaultRootchainInteractor) Call(from types.Address, to types.Address, input []byte) (string, error) {
	return helper.Call(ethgo.Address(from), ethgo.Address(to), input)
}

func (d *defaultRootchainInteractor) SendTransaction(nonce uint64,
	transaction *ethgo.Transaction, signer ethgo.Key) (*ethgo.Receipt, error) {
	return helper.SendTxn(nonce, transaction, signer)
}

func (d *defaultRootchainInteractor) GetPendingNonce(address types.Address) (uint64, error) {
	return helper.GetPendingNonce(address)
}
