package blockbuilder

import (
	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

// TODO: Add open tracing

const (
	BlockGasTargetDivisor uint64 = 1024 // The bound divisor of the gas limit, used in update calculations
)

// BlockBuilderParams are fields for the block that cannot be changed
type BlockBuilderParams struct {
	// Parent block
	Parent *types.Header

	// Executor. TODO: Abstract executor
	Executor *state.Executor

	// Coinbase that is signing the block
	Coinbase types.Address

	// Vanity extra for the block
	VanityExtra []byte

	// GasLimit is the gas limit for the block
	BlockGasTarget uint64

	// Logger
	Logger hclog.Logger

	// txPoolInterface implementation
	TxPool txPoolInterface
}

type blockchainInterface interface {
	// WriteBlock writes the block to the db but does not process it
	WriteBlock(b *types.Block, source string) error
}

type txPoolInterface interface {
	Prepare()
	Peek() *types.Transaction
	Pop(tx *types.Transaction)
	Drop(tx *types.Transaction)
	Demote(tx *types.Transaction)
}

func NewBlockBuilder(params *BlockBuilderParams) (*BlockBuilder, error) {
	// vanity extra can only be 32 size max. it is better to trim that to return
	// an error that we have to propagate. It should be up to higher level
	// code to error if the vanity extra supplied by the user is too big.
	if len(params.VanityExtra) > 32 {
		params.VanityExtra = params.VanityExtra[:32]
	}

	if params.VanityExtra == nil {
		params.VanityExtra = make([]byte, 0)
	}

	if params.Logger == nil {
		params.Logger = hclog.NewNullLogger()
	} else {
		params.Logger = params.Logger.Named("block-builder")
	}

	builder := &BlockBuilder{
		params: params,
	}
	if err := builder.Reset(); err != nil {
		return nil, err
	}

	return builder, nil
}

type BlockBuilder struct {
	// input params for the block
	params *BlockBuilderParams

	// header is the header for the block being created
	header *types.Header

	// transactions are the data included in the block
	txns []*types.Transaction

	// block is a reference to the already built block
	block *types.Block

	state *state.Transition
}

func (b *BlockBuilder) Reset() error {
	// Generate the base block
	header := &types.Header{
		ParentHash: b.params.Parent.Hash,
		Number:     b.params.Parent.Number + 1,
		Miner:      types.ZeroAddress.Bytes(),
		Nonce:      types.Nonce{},
		MixHash:    signer.IstanbulDigest,
		Difficulty: b.params.Parent.Number + 1,
		StateRoot:  types.EmptyRootHash,
		Sha3Uncles: types.EmptyUncleHash,
		GasLimit:   b.params.Parent.GasLimit,
	}

	b.block = nil
	b.txns = []*types.Transaction{}

	// calculate gas limit based on parent header
	header.GasLimit = calculateGasLimit(b.params.Parent.GasLimit, b.params.BlockGasTarget)

	transition, err := b.params.Executor.BeginTxn(b.params.Parent.StateRoot, b.header, b.params.Coinbase)
	if err != nil {
		return err
	}

	b.state = transition

	return nil
}

// Fill fills the block with transactions from the txpool
func (b *BlockBuilder) Fill() error {
	b.params.TxPool.Prepare()

write:
	for {
		tx := b.params.TxPool.Peek()

		// execute transactions one by one
		finished, err := b.writeTxPoolTransaction(tx)
		if err != nil {
			b.params.Logger.Debug("Fill transaction error", "hash", tx.Hash, "err", err)
		}

		if finished {
			break write
		}
	}

	return nil
}

func (b *BlockBuilder) writeTx(tx *types.Transaction) error {
	if err := b.state.Write(tx); err != nil {
		return err
	}

	b.txns = append(b.txns, tx)

	return nil
}

func (b *BlockBuilder) writeTxPoolTransaction(tx *types.Transaction) (bool, error) {
	if tx == nil {
		return true, nil
	}

	if err := b.writeTx(tx); err != nil {
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
			// stop processing
			return true, err
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
			b.params.TxPool.Demote(tx)

			return false, err
		} else {
			b.params.TxPool.Drop(tx)

			return false, err
		}
	}

	// remove tx from the pool and add it to the list of all block transactions
	b.params.TxPool.Pop(tx)

	return false, nil
}

func (b *BlockBuilder) Build(handler func(*types.Header)) *StateBlock {
	// Commit the changes
	_, root := b.state.Commit()

	// Update the header
	b.header.StateRoot = root
	b.header.GasUsed = b.state.TotalGas()

	if handler != nil {
		handler(b.header)
	}

	// Build the actual block
	// The header hash is computed inside buildBlock
	b.block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header: b.header,
		Txns:   b.txns,
		// TODO: Capture the receipts from `state.Write`
		Receipts: b.state.Receipts(),
	})

	b.block.Header.ComputeHash()

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
func calculateGasLimit(parentGasLimit, blockGasTarget uint64) uint64 {
	// The gas limit cannot move more than 1/1024 * parentGasLimit
	// in either direction per block
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
