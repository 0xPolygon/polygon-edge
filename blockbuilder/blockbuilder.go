package blockbuilder

import (
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

const (
	BlockGasTargetDivisor uint64 = 1024 // The bound divisor of the gas limit, used in update calculations
)

type BlockBuilder struct {
	logger hclog.Logger
	txpool *txpool.TxPool
	header *types.Header
	parent *types.Header
}

func (b *BlockBuilder) Reset() {
	// Generate the base block
	header := &types.Header{
		ParentHash: b.parent.Hash,
		Number:     b.parent.Number + 1,
		Miner:      types.ZeroAddress.Bytes(),
		Nonce:      types.Nonce{},
		MixHash:    signer.IstanbulDigest,
		Difficulty: b.parent.Number + 1,
		StateRoot:  types.EmptyRootHash,
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   b.parent.GasLimit,
	}

	// calculate gas limit based on parent header
	gasLimit := b.calculateGasLimit(header.Number)

	header.GasLimit = gasLimit

	miner, err := b.GetBlockCreator(header)
	if err != nil {
		return err
	}

	transition, err := b.executor.BeginTxn(parent.StateRoot, header, miner)
	if err != nil {
		return err
	}
}

func (b *BlockBuilder) Fill() {
	var successful []*types.Transaction

	b.txpool.Prepare()

	for {
		tx := b.txpool.Peek()
		if tx == nil {
			break
		}

		if tx.ExceedsBlockGasLimit(gasLimit) {
			b.txpool.Drop(tx)

			continue
		}

		if err := transition.Write(tx); err != nil {
			if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
				break
			} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
				b.txpool.Demote(tx)
			} else {
				b.txpool.Drop(tx)
			}

			continue
		}

		// no errors, pop the tx from the pool
		b.txpool.Pop(tx)

		successful = append(successful, tx)
	}

	b.logger.Info("picked out txns from pool", "num", len(successful), "remaining", b.txpool.Length())

	return successful
}

func (b *BlockBuilder) Build(callback func(*types.Header)) *StateBlock {
	// Commit the changes
	_, root := transition.Commit()

	// Update the header
	header.StateRoot = root
	header.GasUsed = transition.TotalGas()

	callback(header)

	// Build the actual block
	// The header hash is computed inside buildBlock
	block := consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   header,
		Txns:     txns,
		Receipts: transition.Receipts(),
	})

	if err := b.blockchain.VerifyFinalizedBlock(block); err != nil {
		return err
	}

	// Write the block to the blockchain
	if err := b.blockchain.WriteBlock(block, devConsensus); err != nil {
		return err
	}

	return &StateBlock{
		Block:    b.block,
		Receipts: b.state.Receipts(),
	}
}

// StateBlock is a block with the full state it modifies
type StateBlock struct {
	Block    *types.Block
	Receipts []*types.Receipt
}

// calculateGasLimit calculates gas limit in reference to the block gas target
func (b *BlockBuilder) calculateGasLimit(parentGasLimit uint64) uint64 {
	// The gas limit cannot move more than 1/1024 * parentGasLimit
	// in either direction per block
	blockGasTarget := b.Config().BlockGasTarget

	// Check if the gas limit target has been set
	if blockGasTarget == 0 {
		// The gas limit target has not been set,
		// so it should use the parent gas limit
		return parentGasLimit
	}

	// Check if the gas limit is already at the target
	if parentGasLimit == blockGasTarget {
		// The gas limit is already at the target, no need to move it
		return blockGasTarget
	}

	delta := parentGasLimit * 1 / BlockGasTargetDivisor
	if parentGasLimit < blockGasTarget {
		// The gas limit is lower than the gas target, so it should
		// increase towards the target
		return common.Min(blockGasTarget, parentGasLimit+delta)
	}

	// The gas limit is higher than the gas target, so it should
	// decrease towards the target
	return common.Max(blockGasTarget, common.Max(parentGasLimit-delta, 0))
}
