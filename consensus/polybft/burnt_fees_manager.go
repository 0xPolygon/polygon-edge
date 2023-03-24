package polybft

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/chain"

	"github.com/armon/go-metrics"
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

// dummyBurntFeesManager is the dummy implementation of BurntFeesManager
type dummyBurntFeesManager struct {
}

func (m *dummyBurntFeesManager) PostBlock(req *PostBlockRequest) error {
	return nil
}

// burntFeesManager implements BurntFeesManager interface by implementing a logic that
// sends the withdrawal transaction to the EIP1559Burnt contract once an epoch.
// This is needed to automatically sync burnt fees between root chain and child chain.
type burntFeesManager struct {
	// key is the identity of the node submitting a checkpoint
	key ethgo.Key

	// txRelayer is the abstraction on the child chain interaction logic.
	txRelayer txrelayer.TxRelayer

	// calculateBurnContract returns burnt contract address based on the given block
	calculateBurnContract func(block uint64) (types.Address, error)

	// forks is needed to check if london hardfork is enabled for the given block
	forks *chain.Forks

	// checkpointsOffset represents offset between withdrawal blocks (applicable only for non-epoch ending blocks)
	withdrawalOffset uint64

	// lastWithdrawalBlock represents the last block on which a withdrawal transaction was sent
	lastWithdrawalBlock uint64

	// logger is the default logger
	logger hclog.Logger
}

// newBurntFeesManager is the constructor of burntFeesManager
func newBurntFeesManager(
	key ethgo.Key,
	txRelayer txrelayer.TxRelayer,
	withdrawalOffset uint64,
	calculateBurnContract func(block uint64) (types.Address, error),
	forks *chain.Forks,
	logger hclog.Logger,
) *burntFeesManager {
	return &burntFeesManager{
		key:                   key,
		txRelayer:             txRelayer,
		calculateBurnContract: calculateBurnContract,
		forks:                 forks,
		withdrawalOffset:      withdrawalOffset,
		logger:                logger,
	}
}

func (m *burntFeesManager) PostBlock(req *PostBlockRequest) error {
	latestHeader := req.FullBlock.Block.Header

	// Check if london hardfork is enabled
	if !m.forks.IsLondon(latestHeader.Number) {
		return nil
	}

	// Check if withdrawal is needed by the current miner
	if !m.isWithdrawalBlock(latestHeader.Number, req.IsEpochEndingBlock) ||
		!bytes.Equal(m.key.Address().Bytes(), latestHeader.Miner) {
		return nil
	}

	m.logger.Debug("burnt fees withdrawal invoked...",
		"withdrawal block", latestHeader.Number)

	// Get burn contract address based on the given header
	burnContractAddr, err := m.calculateBurnContract(latestHeader.Number)
	if err != nil {
		return err
	}

	burnContractAddrPtr := ethgo.Address(burnContractAddr)
	txn := &ethgo.Transaction{
		To:   &burnContractAddrPtr,
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

	// Update last withdrawal block
	m.lastWithdrawalBlock = req.FullBlock.Block.Number()

	// update burnt fees withdrawal block number metrics
	metrics.SetGauge([]string{"bridge", "checkpoint_block_number"}, float32(latestHeader.Number))
	m.logger.Debug("successfully sent burnt fees withdrawal tx", "block number", latestHeader.Number)

	return nil
}

// isWithdrawalBlock returns true for blocks in the middle of the epoch
// which are offset by predefined count of blocks or if given block is an epoch ending block
func (m *burntFeesManager) isWithdrawalBlock(blockNumber uint64, isEpochEndingBlock bool) bool {
	return isEpochEndingBlock || blockNumber == m.lastWithdrawalBlock+m.withdrawalOffset
}
