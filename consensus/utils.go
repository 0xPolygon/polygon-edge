package consensus

import (
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
)

// BuildBlockParams are parameters passed into the BuildBlock helper method
type BuildBlockParams struct {
	Header   *types.Header
	Txns     []*types.Transaction
	Receipts []*types.Receipt
}

// BuildBlock is a utility function that builds a block, based on the passed in header, transactions and receipts
func BuildBlock(params BuildBlockParams) *types.Block {
	txs := params.Txns
	receipts := params.Receipts
	header := params.Header

	if len(txs) == 0 {
		header.TxRoot = types.EmptyRootHash
	} else {
		header.TxRoot = buildroot.CalculateTransactionsRoot(txs)
	}

	if len(receipts) == 0 {
		header.ReceiptsRoot = types.EmptyRootHash
	} else {
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(receipts)
	}

	// TODO: Compute uncles
	header.Sha3Uncles = types.EmptyUncleHash
	header.ComputeHash()

	return &types.Block{
		Header:       header,
		Transactions: txs,
	}
}
