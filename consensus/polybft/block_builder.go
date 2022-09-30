package polybft

import (
	"time"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state"
	"github.com/0xPolygon/polygon-edge/types"
)

// TODO: Add opentracing

// type txStatus uint8

// const (
// 	success txStatus = iota
// 	fail
// 	skip
// 	nothing
// )

// type txPoolInterface interface {
// 	Prepare()
// 	Length() uint64
// 	Peek() *types.Transaction
// 	Pop(tx *types.Transaction)
// 	Drop(tx *types.Transaction)
// 	Demote(tx *types.Transaction)
// 	ResetWithHeaders(headers ...*types.Header)
// 	SetSealing(bool)
// }

// type txEvmTransition interface {
// 	Write(txn *types.Transaction) error
// 	WriteFailedReceipt(txn *types.Transaction) error
// }

// BlockBuilderParams are fields for the block that cannot be changed
type BlockBuilderParams struct {
	// Parent block
	Parent *types.Header

	State *state.Transition

	// Coinbase that is signing the block
	Coinbase types.Address

	// ChainConfig is the configurtion of the chain
	ChainConfig *chain.Params

	// ChainContext interface is used during EVM execution
	//ChainContext core.ChainContext

	// Vanity extra for the block
	Extra []byte

	// Executor for EVM execution
	//Transition txEvmTransition

	// GasLimit is the gas limit for the block
	GasLimit uint64

	// duration for one block
	BlockTime time.Duration

	// Logger
	//Logger hcf.Logger

	// TxPool txPoolInterface // Reference to the transaction pool
}

func NewBlockBuilder(params *BlockBuilderParams) *BlockBuilder {
	// extra can only be 32 size max. it is better to trim that to return
	// an error that we have to propagate. It should be up to higher level
	// code to error if the extra supplied by the user is too big.
	if len(params.Extra) > 32 {
		params.Extra = params.Extra[:32]
	}
	if params.Extra == nil {
		params.Extra = make([]byte, 0)
	}
	if params.BlockTime == 0 {
		params.BlockTime = time.Second * 30 // TODO: is this ok?
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

	// transactions and receipts are the data included in the block
	txns     []*types.Transaction
	receipts []*types.Receipt

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
		//BaseFee:    big.NewInt(100), // TODO: what is base fee
	}

	b.block = nil
	b.txns = []*types.Transaction{}
	b.receipts = []*types.Receipt{}

	// b.signer = types.MakeSigner(b.params.ChainConfig, b.header.Number)

	// should this be copied? probably not
	b.state = b.params.State

	b.state = &state.Transition{} // TODO: build snapshot somehow from  b.params (by using state.NewSnapshotAt?)
}

// Block returns the built block if nil, it is not built yet
func (b *BlockBuilder) Block() *types.Block {
	return b.block
}

// Build creates the state and the final block
func (b *BlockBuilder) Build(handler func(h *types.Header)) *StateBlock {
	if handler != nil {
		handler(b.header)
	}
	// build the block and write the state
	// b.header.Root = b.state.IntermediateRoot(true)
	b.header.StateRoot = types.Hash{} // set somehow state root with executor or something else after Ferran's changes
	b.block = NewFinalBlock(b.header, b.txns, b.receipts)

	// TO DO Nemanja - see what to do with gas later
	// calculate gas limit based on parent header
	// gasLimit, err := b.blockchain.CalculateGasLimit(header.Number)
	// if err != nil {
	// 	return nil, err
	// }

	// header.GasLimit = gasLimit

	return &StateBlock{
		Block:    b.block,
		Receipts: b.receipts,
		State:    nil, // TO DO Nemanja - fix this somehow
	}
}

/*
// Fill fills the block with transactions from the txpool
func (b *BlockBuilder) Fill() (successful int, failed int, skipped int, timeout bool) {
	executed := make([]*types.Transaction, 0)

	var blockTimer = time.NewTimer(b.params.BlockTime)

	defer func() {
		b.params.Logger.Info(
			"executed txs",
			"successful", successful,
			"failed", failed,
			"skipped", skipped,
			"remaining", b.params.TxPool.Length(),
		)
	}()

	b.params.TxPool.Prepare()

write:
	for {
		select {
		case <-blockTimer.C:
			return successful, failed, skipped, true
		default:
			tx := b.params.TxPool.Peek()
			// execute transactions one by one
			result, ok := b.writeTransaction(tx)

			if !ok {
				break write
			}

			switch result {
			case success:
				executed = append(executed, tx)
				successful++
			case fail:
				failed++
			case skip:
				skipped++
			}
		}
	}

	//	wait for the timer to expire
	<-blockTimer.C

	return successful, failed, skipped, false
}

func (b *BlockBuilder) writeTransaction(
	tx *types.Transaction) (txstat txStatus, continueProcessing bool) {
	if tx == nil {
		return skip, false
	}

	if tx.ExceedsBlockGasLimit(b.params.GasLimit) {
		b.params.TxPool.Drop(tx)

		if err := b.params.Transition.WriteFailedReceipt(tx); err != nil {
			b.params.Logger.Error(
				fmt.Sprintf(
					"unable to write failed receipt for transaction %s",
					tx.Hash,
				),
			)
		}

		// continue processing
		return fail, true
	}

	if err := b.params.Transition.Write(tx); err != nil {
		if _, ok := err.(*state.GasLimitReachedTransitionApplicationError); ok { //nolint:errorlint
			// stop processing
			return nothing, false
		} else if appErr, ok := err.(*state.TransitionApplicationError); ok && appErr.IsRecoverable { //nolint:errorlint
			b.params.TxPool.Demote(tx)

			return skip, true
		} else {
			b.params.TxPool.Drop(tx)

			return fail, true
		}
	}

	b.params.TxPool.Pop(tx)

	return success, true
}
*/

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

func NewFinalBlock(header *types.Header, txs []*types.Transaction, receipts []*types.Receipt) *types.Block {
	b := &types.Block{Header: header.Copy()}
	if len(txs) != len(receipts) {
		panic("cannot create block: number of receipts and txs is not the same")
	}

	// TO DO Nemanja - there are no thx and receipts, so leve everithing as default

	return b
}
