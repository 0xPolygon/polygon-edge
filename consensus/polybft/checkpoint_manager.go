package polybft

import (
	"fmt"
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
		"uint256 epochNumber, uint256 blockNumber, bytes32 blockHash, uint256 blockRound" +
		"bytes32 eventRoot, tuple(address _address, uint256[4] blsKey)[] nextValidators" + ")")
)

type checkpointManager struct {
	sender types.Address
}

// getCurrentCheckpointID queries CheckpointManager smart contract and retrieves current checkpoint id
func (c checkpointManager) getCurrentCheckpointID(epochNumber uint64) (uint64, error) {
	checkpointIDMethodEncoded, err := currentCheckpointIDMethod.Encode([]interface{}{})
	if err != nil {
		return 0, fmt.Errorf("failed to encode currentCheckpointId function invocation for epoch=%d: %w",
			epochNumber, err)
	}

	currentCheckpointID, err := helper.Call(ethgo.Address(c.sender),
		ethgo.Address(helper.CheckpointManagerAddress), checkpointIDMethodEncoded)
	if err != nil {
		return 0, fmt.Errorf("failed to invoke currentCheckpointId on the rootchain for epoch=%d: %w",
			epochNumber, err)
	}

	checkpointID, err := strconv.ParseUint(currentCheckpointID, 0, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to convert current checkpoint id '%s' to number: %w", currentCheckpointID, err)
	}

	return checkpointID, nil
}

// submitCheckpoint sends a transaction which contains checkpoint data to the rootchain
func (c *checkpointManager) submitCheckpoint(header *types.Header) error {
	extra, err := GetIbftExtra(header.ExtraData)
	if err != nil {
		return err
	}

	// TODO: REMOVE
	fmt.Println(extra.Checkpoint)

	// TODO: Spawn a routine
	// TODO: Encode input
	checkpointManagerAddr := ethgo.Address(helper.CheckpointManagerAddress)
	txn := &ethgo.Transaction{
		To: &checkpointManagerAddr,
	}
	pendingNonce, err := helper.GetPendingNonce(c.sender)

	if err != nil {
		return err
	}

	helper.SendTxn(pendingNonce, txn)

	return nil
}
