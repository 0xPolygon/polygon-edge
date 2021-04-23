package consensus

import (
	"github.com/0xPolygon/minimal/types"
	"github.com/0xPolygon/minimal/types/buildroot"
)

func BuildBlock(header *types.Header, txs []*types.Transaction, receipts []*types.Receipt) *types.Block {
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
