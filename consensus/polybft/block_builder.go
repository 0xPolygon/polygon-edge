package polybft

import (
	"time"

	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/txpool"
	"github.com/0xPolygon/polygon-edge/types"
	hcf "github.com/hashicorp/go-hclog"
)

// TODO: Add opentracing

// BlockBuilderParams are fields for the block that cannot be changed
type BlockBuilderParams struct {
	// Parent block
	Parent *types.Header

	// Executor
	Executor *state.Executor

	// Coinbase that is signing the block
	Coinbase types.Address

	// Vanity extra for the block
	Extra []byte

	// GasLimit is the gas limit for the block
	GasLimit uint64

	// duration for one block
	BlockTime time.Duration

	// Logger
	Logger hcf.Logger

	// txPoolInterface implementation
	TxPool txPoolInterface
}

func NewBlockBuilder(params *BlockBuilderParams) (*BlockBuilder, error) {
	// extra can only be 32 size max. it is better to trim that to return
	// an error that we have to propagate. It should be up to higher level
	// code to error if the extra supplied by the user is too big.
	if len(params.Extra) > signer.IstanbulExtraVanity {
		params.Extra = params.Extra[:signer.IstanbulExtraVanity]
	}

	if params.Extra == nil {
		params.Extra = make([]byte, 0)
	}

	builder := &BlockBuilder{
		params: params,
	}

	if err := builder.Reset(); err != nil {
		return nil, err
	}

	return builder, nil
}

var _ blockBuilder = &BlockBuilder{}

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

// Reset is used to indicate that the current block building has been interrupted
// and it has to clean any data
func (b *BlockBuilder) Reset() error {
	b.header = &types.Header{
		ParentHash:   b.params.Parent.Hash,
		Number:       b.params.Parent.Number + 1,
		Miner:        b.params.Coinbase[:],
		Difficulty:   1,
		ExtraData:    b.params.Extra,
		StateRoot:    types.EmptyRootHash, // this avoids needing state for now
		TxRoot:       types.EmptyRootHash,
		ReceiptsRoot: types.EmptyRootHash, // this avoids needing state for now
		Sha3Uncles:   types.EmptyUncleHash,
		GasLimit:     b.params.GasLimit,
	}

	b.block = nil
	b.txns = []*types.Transaction{}

	transition, err := b.params.Executor.BeginTxn(b.params.Parent.StateRoot, b.header, b.params.Coinbase)
	if err != nil {
		return err
	}

	b.state = transition

	return nil
}

// Block returns the built block if nil, it is not built yet
func (b *BlockBuilder) Block() *types.Block {
	return b.block
}

// Build creates the state and the final block
func (b *BlockBuilder) Build(handler func(h *types.Header)) (*StateBlock, error) {
	if handler != nil {
		handler(b.header)
	}

	_, b.header.StateRoot = b.state.Commit()
	b.header.GasUsed = b.state.TotalGas()

	// build the block
	b.block = consensus.BuildBlock(consensus.BuildBlockParams{
		Header:   b.header,
		Txns:     b.txns,
		Receipts: b.state.Receipts(),
	})

	b.block.Header.ComputeHash()

	return &StateBlock{
		Block:    b.block,
		Receipts: b.state.Receipts(),
		State:    b.state,
	}, nil
}

func (b *BlockBuilder) WriteTx(tx *types.Transaction) error {
	if tx.ExceedsBlockGasLimit(b.params.GasLimit) {
		if err := b.state.WriteFailedReceipt(tx); err != nil {
			return err
		}

		return txpool.ErrBlockLimitExceeded
	}

	if err := b.state.Write(tx); err != nil {
		return err
	}

	b.txns = append(b.txns, tx)

	return nil
}

// Fill fills the block with transactions from the txpool
func (b *BlockBuilder) Fill() error {
	blockTimer := time.NewTimer(b.params.BlockTime)

	b.params.TxPool.Prepare()
write:
	for {
		select {
		case <-blockTimer.C:
			return nil
		default:
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
	}

	//	wait for the timer to expire
	<-blockTimer.C

	return nil
}

// CommitTransaction applies given transaction to the state. If transaction apply fails, it reverts the saved snapshot.
func (b *BlockBuilder) CommitTransaction(tx *types.Transaction) error {
	if tx.ExceedsBlockGasLimit(b.params.GasLimit) {
		if err := b.state.WriteFailedReceipt(tx); err != nil {
			return err
		}

		return txpool.ErrBlockLimitExceeded
	}

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

	err := b.CommitTransaction(tx)
	if err != nil {
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

// GetState returns Transition reference
func (b *BlockBuilder) GetState() *state.Transition {
	return b.state
}

// StateBlock is a block with the full state it modifies
type StateBlock struct {
	Block    *types.Block
	Receipts []*types.Receipt
	State    *state.Transition
}
