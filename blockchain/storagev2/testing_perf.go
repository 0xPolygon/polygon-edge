package storagev2

import (
	"crypto/rand"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
)

const letterBytes = "abcdef0123456789"

func randStringBytes(t *testing.T, n int) string {
	t.Helper()

	b := make([]byte, n)
	_, err := rand.Reader.Read(b)
	require.NoError(t, err)

	return string(b)
}

func createTxs(t *testing.T, startNonce, count int, from types.Address, to *types.Address) []*types.Transaction {
	t.Helper()

	txs := make([]*types.Transaction, count)

	for i := range txs {
		tx := types.NewTx(types.NewDynamicFeeTx(
			types.WithGas(types.StateTransactionGasLimit),
			types.WithNonce(uint64(startNonce+i)),
			types.WithFrom(from),
			types.WithTo(to),
			types.WithValue(big.NewInt(2000)),
			types.WithGasFeeCap(big.NewInt(100)),
			types.WithGasTipCap(big.NewInt(10)),
		))

		txs[i] = tx
	}

	return txs
}

func CreateBlock(t *testing.T) *types.FullBlock {
	t.Helper()

	transactionsCount := 2500
	status := types.ReceiptSuccess
	addr1 := types.StringToAddress("17878aa")
	addr2 := types.StringToAddress("2bf5653")
	b := &types.FullBlock{
		Block: &types.Block{
			Header: &types.Header{
				Number:    0,
				ExtraData: make([]byte, 32),
				Hash:      types.ZeroHash,
			},
			Transactions: createTxs(t, 0, transactionsCount, addr1, &addr2),
			// Uncles:       blockchain.NewTestHeaders(10),
		},
		Receipts: make([]*types.Receipt, transactionsCount),
	}

	logs := make([]*types.Log, 10)

	for i := 0; i < 10; i++ {
		logs[i] = &types.Log{
			Address: addr1,
			Topics:  []types.Hash{types.StringToHash("t1"), types.StringToHash("t2"), types.StringToHash("t3")},
			Data:    []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xbb, 0xaa, 0x01, 0x012},
		}
	}

	for i := 0; i < len(b.Block.Transactions); i++ {
		b.Receipts[i] = &types.Receipt{
			TxHash:            b.Block.Transactions[i].Hash(),
			Root:              types.StringToHash("mockhashstring"),
			TransactionType:   types.LegacyTxType,
			GasUsed:           uint64(100000),
			Status:            &status,
			Logs:              logs,
			CumulativeGasUsed: uint64(100000),
			ContractAddress:   &types.Address{0xaa, 0xbb, 0xcc, 0xdd, 0xab, 0xac},
		}
	}

	for i := 0; i < 5; i++ {
		b.Receipts[i].LogsBloom = types.CreateBloom(b.Receipts)
	}

	return b
}

func UpdateBlock(t *testing.T, num uint64, b *types.FullBlock) *types.FullBlock {
	t.Helper()

	var addr types.Address

	b.Block.Header.Number = num
	b.Block.Header.ParentHash = types.StringToHash(randStringBytes(t, 12))

	for i := range b.Block.Transactions {
		addr = types.StringToAddress(randStringBytes(t, 8))
		b.Block.Transactions[i].SetTo(&addr)
		b.Block.Transactions[i].ComputeHash()
		b.Receipts[i].TxHash = b.Block.Transactions[i].Hash()
	}

	b.Block.Header.ComputeHash()

	return b
}

func PrepareBatch(t *testing.T, s *Storage, b *types.FullBlock) *Writer {
	t.Helper()

	batchWriter := s.NewWriter()

	// Lookup 'sorted'
	batchWriter.PutHeadHash(b.Block.Header.Hash)
	batchWriter.PutHeadNumber(b.Block.Number())
	batchWriter.PutBlockLookup(b.Block.Hash(), b.Block.Number())

	for _, tx := range b.Block.Transactions {
		batchWriter.PutTxLookup(tx.Hash(), b.Block.Number())
	}

	// Main DB sorted
	batchWriter.PutBody(b.Block.Number(), b.Block.Hash(), b.Block.Body())
	batchWriter.PutCanonicalHash(b.Block.Number(), b.Block.Hash())
	batchWriter.PutHeader(b.Block.Header)
	batchWriter.PutReceipts(b.Block.Number(), b.Block.Hash(), b.Receipts)

	return batchWriter
}
