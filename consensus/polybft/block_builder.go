package polybft

import (
	"fmt"
	"time"

	"github.com/0xPolygon/polygon-edge/consensus"
	"github.com/0xPolygon/polygon-edge/consensus/ibft/signer"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
	hcf "github.com/hashicorp/go-hclog"
)

// TODO: Add opentracing

type txWriteStatus uint8

const (
	success txWriteStatus = iota
	fail
	skip
	finishProcessing
)

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

func NewBlockBuilder(params *BlockBuilderParams) *BlockBuilder {
	// extra can only be 32 size max. it is better to trim that to return
	// an error that we have to propagate. It should be up to higher level
	// code to error if the extra supplied by the user is too big.
	if len(params.Extra) > signer.IstanbulExtraVanity {
		params.Extra = params.Extra[:signer.IstanbulExtraVanity]
	}

	if params.Extra == nil {
		params.Extra = make([]byte, 0)
	}

	if params.BlockTime == 0 {
		params.BlockTime = time.Second * 2 // TODO: is this ok?
	}

	builder := &BlockBuilder{
		params: params,
	}
	builder.Reset()

	return builder
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
func (b *BlockBuilder) Reset() {
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
		GasLimit:     b.params.GasLimit, //will need to adjust dynamically later.
	}

	b.block = nil
	b.txns = []*types.Transaction{}

	transition, err := b.params.Executor.BeginTxn(b.params.Parent.StateRoot, b.header, b.params.Coinbase)
	if err != nil {
		panic(err)
	}

	b.state = transition
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

// Fill fills the block with transactions from the txpool
func (b *BlockBuilder) Fill() error {
	successful, failed, skipped := 0, 0, 0
	blockTimer := time.NewTimer(b.params.BlockTime)
	timeout := false

	defer func() {
		b.params.Logger.Debug(
			"executed txs",
			"successful", successful,
			"failed", failed,
			"skipped", skipped,
			"timeout", timeout,
			"remaining", b.params.TxPool.Length(),
		)
	}()

	b.params.TxPool.Prepare()
write:
	for {
		select {
		case <-blockTimer.C:
			timeout = true
			if successful == 0 && failed > 0 {
				return fmt.Errorf("block builder fill fail: failed = %d", failed)
			}

			return nil
		default:
			tx := b.params.TxPool.Peek()

			// execute transactions one by one
			switch b.writeTransaction(tx) {
			case success:
				b.txns = append(b.txns, tx)
				successful++
			case fail:
				failed++
			case skip:
				skipped++
			case finishProcessing:
				break write
			}
		}
	}

	//	wait for the timer to expire
	<-blockTimer.C

	if successful == 0 && failed > 0 {
		return fmt.Errorf("block builder fill fail: failed = %d", failed)
	}

	return nil
}

func (b *BlockBuilder) writeTransaction(tx *types.Transaction) txWriteStatus {
	if tx == nil {
		return finishProcessing
	}

	if tx.ExceedsBlockGasLimit(b.params.GasLimit) {
		b.params.TxPool.Drop(tx)

		if err := b.state.WriteFailedReceipt(tx); err != nil {
			b.params.Logger.Error("unable to write failed receipt for transaction",
				"hash", tx.Hash)
		}

		// continue processing
		return fail
	}

	if err := b.state.Write(tx); err != nil {
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
			// stop processing
			return finishProcessing
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
			b.params.TxPool.Demote(tx)

			return skip
		} else {
			b.params.TxPool.Drop(tx)

			return fail
		}
	}

	b.params.TxPool.Pop(tx)

	return success
}

// GetState returns StateDB reference
func (b *BlockBuilder) GetState() *state.Transition {
	return b.state
}

// StateBlock is a block with the full state it modifies
type StateBlock struct {
	Block    *types.Block
	Receipts []*types.Receipt
	State    *state.Transition
}
