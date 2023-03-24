package polybft

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"

	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/txrelayer"
	"github.com/0xPolygon/polygon-edge/types"
)

const (
	// defaultBurntFeesWithdrawalOffset is the frequency at which withdrawals are sent to the rootchain (in blocks)
	defaultBurntFeesWithdrawalOffset = uint64(900)
)

type BurntFeesManager interface {
	PostBlock(req *PostBlockRequest) error
}

type dummyBurntFeesManager struct {
}

func (m *dummyBurntFeesManager) PostBlock(req *PostBlockRequest) error {
	return nil
}

type burntFeesManager struct {
	// key is the identity of the node submitting a checkpoint
	key ethgo.Key

	// txRelayer is the abstraction on the child chain interaction logic.
	txRelayer txrelayer.TxRelayer

	// burntFeesManagerAddr is the address of EIP-1599 burnt contract
	burntFeesManagerAddr types.Address

	// checkpointsOffset represents offset between withdrawal blocks (applicable only for non-epoch ending blocks)
	withdrawalOffset uint64

	// lastWithdrawalBlock represents the last block on which a withdrawal transaction was sent
	lastWithdrawalBlock uint64

	// logger is the default logger
	logger hclog.Logger
}

func newBurntFeesManager(
	key ethgo.Key,
	txRelayer txrelayer.TxRelayer,
	withdrawalOffset uint64,
	burntFeesManagerAddr types.Address,
	logger hclog.Logger,
) *burntFeesManager {
	return &burntFeesManager{
		key:                  key,
		txRelayer:            txRelayer,
		burntFeesManagerAddr: burntFeesManagerAddr,
		withdrawalOffset:     withdrawalOffset,
		logger:               logger,
	}
}

func (m *burntFeesManager) PostBlock(req *PostBlockRequest) error {
	latestHeader := req.FullBlock.Block.Header

	m.logger.Debug("burnt fees withdrawal invoked...",
		"withdrawal block", latestHeader.Number)

	burntFeesManagerAddr := ethgo.Address(m.burntFeesManagerAddr)
	txn := &ethgo.Transaction{
		To:   &burntFeesManagerAddr,
		From: m.key.Address(),
	}

	// Encode transaction input
	input, err := contractsapi.EIP1559Burn.Abi.GetMethod("withdraw").Encode([]interface{}{})
	if err != nil {
		return err
	}

	txn.Input = input

	// Send withdrawal transaction
	receipt, err := m.txRelayer.SendTransaction(txn, m.key)
	if err != nil {
		return err
	}

	if receipt.Status == uint64(types.ReceiptFailed) {
		return fmt.Errorf("burnt fees withdrawal transaction failed for block %d", latestHeader.Number)
	}

	// update burnt fees withdrawal block number metrics
	// metrics.SetGauge([]string{"bridge", "checkpoint_block_number"}, float32(header.Number))
	// m.logger.Debug("successfully sent burnt fees withdrawal tx", "block number", header.Number)

	return nil
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offset by predefined count of blocks or if given block is an epoch ending block
func (m *burntFeesManager) isCheckpointBlock(blockNumber uint64, isEpochEndingBlock bool) bool {
	return isEpochEndingBlock || blockNumber == m.lastWithdrawalBlock+m.withdrawalOffset
}
