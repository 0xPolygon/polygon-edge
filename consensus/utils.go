package consensus

import (
	"time"

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
		header.TxRoot = buildroot.CalculateTransactionsRoot(txs)
	}

	if len(params.Receipts) == 0 {
		header.ReceiptsRoot = types.EmptyRootHash
	} else {
		header.ReceiptsRoot = buildroot.CalculateReceiptsRoot(params.Receipts)
	}

	// TODO: Compute uncles
	header.Sha3Uncles = types.EmptyUncleHash
	header.ComputeHash()

	return &types.Block{
		Header:       header,
		Transactions: txs,
	}
}

// MilliToUnix returns the local Time corresponding to the given Unix time m milliseconds since January 1, 1970 UTC.
func MilliToUnix(m uint64) time.Time {
	return time.Unix(0, int64(m)*1e6)
}

// UnixToMilli returns uint64 value for miliseconds
func UnixToMilli(t time.Time) uint64 {
	return uint64(t.UnixNano() / 1e6)
}
