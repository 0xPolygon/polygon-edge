package polybft

import (
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/umbracle/ethgo"
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
	withdrawalOffset uint64,
	burntFeesManagerAddr types.Address,
	logger hclog.Logger,
) *burntFeesManager {
	return &burntFeesManager{
		key:                  key,
		burntFeesManagerAddr: burntFeesManagerAddr,
		withdrawalOffset:     withdrawalOffset,
		logger:               logger,
	}
}

func (m *burntFeesManager) PostBlock(req *PostBlockRequest) error {
	m.logger.Debug("burnt fees withdrawal invoked...",
		"latest withdrawal block", lastCheckpointBlockNumber,
		"withdrawal block", latestHeader.Number)

	checkpointManagerAddr := ethgo.Address(m.burntFeesManagerAddr)
	txn := &ethgo.Transaction{
		To:   &checkpointManagerAddr,
		From: m.key.Address(),
	}

	input, err := contractsapi.EIP1559Burn.Abi.GetMethod("withdraw").Encode([]interface{}{})
	if err != nil {
		return err
	}

	txn.Input = input

	panic("not implemented yet")
}

// isCheckpointBlock returns true for blocks in the middle of the epoch
// which are offset by predefined count of blocks or if given block is an epoch ending block
func (m *burntFeesManager) isCheckpointBlock(blockNumber uint64, isEpochEndingBlock bool) bool {
	return isEpochEndingBlock || blockNumber == m.lastWithdrawalBlock+m.withdrawalOffset
}
