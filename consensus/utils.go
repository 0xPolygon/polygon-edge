package consensus

import (
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/0xPolygon/polygon-edge/types/buildroot"
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
	header := params.Header

	if len(txs) == 0 {
		header.TxRoot = types.EmptyRootHash
	} else {
		header.TxRoot = buildroot.CalculateTransactionsRoot(txs, header.Number)
	}

	if len(params.Receipts) == 0 {
		header.ReceiptsRoot = types.EmptyRootHash
	} else {
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(params.Receipts)
	}

	header.Sha3Uncles = types.EmptyUncleHash
	header.ComputeHash()

	return &types.Block{
		Header:       header,
		Transactions: txs,
	}
}
