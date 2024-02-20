package buildroot

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestCalculateReceiptsRoot(t *testing.T) {
	t.Parallel()

	statusSuccess := types.ReceiptSuccess
	statusFailed := types.ReceiptFailed
	contractAddr1 := types.StringToAddress("0x3")
	contractAddr2 := types.StringToAddress("0x4")

	t.Run("fast rlp", func(t *testing.T) {
		t.Parallel()

		receipts := []*types.Receipt{
			{
				TxHash:            types.StringToHash("0x1"),
				Root:              types.StringToHash("0x2"),
				Status:            &statusSuccess,
				CumulativeGasUsed: 100,
				GasUsed:           70,
				ContractAddress:   &contractAddr1,
				TransactionType:   types.DynamicFeeTx,
				Logs: []*types.Log{
					{
						Address: contractAddr1,
						Topics: []types.Hash{
							types.StringToHash("0x11"),
							types.StringToHash("0x22"),
							types.StringToHash("0x33"),
						},
						Data: []byte{0x1, 0x2, 0x3},
					},
				},
			},
			{
				TxHash:            types.StringToHash("0x3"),
				Root:              types.StringToHash("0x4"),
				Status:            &statusFailed,
				CumulativeGasUsed: 100,
				GasUsed:           30,
				ContractAddress:   &contractAddr2,
				TransactionType:   types.LegacyTx,
				Logs: []*types.Log{
					{
						Address: contractAddr1,
						Topics: []types.Hash{
							types.StringToHash("0x111"),
							types.StringToHash("0x222"),
							types.StringToHash("0x333"),
						},
						Data: []byte{0x11, 0x21, 0x31},
					},
				},
			},
		}

		expectedRoot := types.StringToHash("0x7e97be0b1473b8486d553256573cde4fb0b52cf2c76b57375a907227ed22bfee")

		root := CalculateReceiptsRoot(receipts)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})

	t.Run("slow rlp", func(t *testing.T) {
		t.Parallel()

		receipts := make([]*types.Receipt, 0, 130)

		for i := uint64(0); i < 130; i++ {
			receipts = append(receipts, &types.Receipt{
				TxHash:            types.StringToHash("0x1"),
				Root:              types.StringToHash("0x2"),
				Status:            &statusSuccess,
				CumulativeGasUsed: 100 + i,
				GasUsed:           70 + i,
				ContractAddress:   &contractAddr1,
				TransactionType:   types.DynamicFeeTx,
				Logs: []*types.Log{
					{
						Address: contractAddr1,
						Topics: []types.Hash{
							types.StringToHash("0x11"),
							types.StringToHash("0x22"),
							types.StringToHash("0x33"),
						},
						Data: []byte{0x1, 0x2, 0x3},
					},
				},
			})
		}

		expectedRoot := types.StringToHash("0x4741f0241bb17da5d9584d3e860669d60d3a5322de90dbc1a9b8c541221fbb1b")

		root := CalculateReceiptsRoot(receipts)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})
}

func TestCalculateTransactionsRoot(t *testing.T) {
	t.Parallel()

	t.Run("no transactions", func(t *testing.T) {
		t.Parallel()

		transactions := []*types.Transaction{}
		blockNumber := uint64(12345)
		expectedRoot := types.EmptyRootHash

		root := CalculateTransactionsRoot(transactions, blockNumber)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})

	t.Run("has transactions", func(t *testing.T) {
		t.Parallel()

		contractAddr1 := types.StringToAddress("0x3")
		contractAddr2 := types.StringToAddress("0x4")

		transactions := []*types.Transaction{
			{
				Inner: &types.MixedTxn{
					Hash:      types.StringToHash("0x1"),
					From:      types.StringToAddress("0x2"),
					To:        &contractAddr1,
					Value:     big.NewInt(100),
					GasTipCap: big.NewInt(10),
					GasFeeCap: big.NewInt(100),
					Input:     []byte{0x1, 0x2, 0x3},
					Nonce:     1,
					Gas:       100000,
					ChainID:   big.NewInt(1),
					Type:      types.DynamicFeeTx,
				},
			},
			{
				Inner: &types.MixedTxn{
					Hash:     types.StringToHash("0x4"),
					From:     types.StringToAddress("0x5"),
					To:       &contractAddr2,
					Value:    big.NewInt(200),
					GasPrice: big.NewInt(20),
					Gas:      200000,
					Input:    []byte{0x4, 0x5, 0x6},
					Nonce:    2,
					Type:     types.LegacyTx,
				},
			},
		}

		blockNumber := uint64(12345)
		expectedRoot := types.StringToHash("0x952361609fb9c56b6af2ad9c37818a4e19f166fbcc8efe7bee91aed4aaaa6bc2")

		root := CalculateTransactionsRoot(transactions, blockNumber)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})
}

func TestCalculateUncleRoot(t *testing.T) {
	t.Parallel()

	t.Run("has uncles", func(t *testing.T) {
		t.Parallel()

		uncles := []*types.Header{
			{
				ParentHash:   types.StringToHash("0x1"),
				Sha3Uncles:   types.StringToHash("0x2"),
				Miner:        types.StringToAddress("0x3").Bytes(),
				StateRoot:    types.StringToHash("0x4"),
				TxRoot:       types.StringToHash("0x5"),
				ReceiptsRoot: types.StringToHash("0x6"),
				Difficulty:   0,
				Number:       1,
				GasLimit:     100000,
				GasUsed:      70000,
				Timestamp:    1626361200,
				ExtraData:    []byte{0x1, 0x2, 0x3},
				MixHash:      types.StringToHash("0x7"),
				Nonce:        types.Nonce{1},
				Hash:         types.StringToHash("0x8"),
				BaseFee:      100,
			},
			{
				ParentHash:   types.StringToHash("0x9"),
				Sha3Uncles:   types.StringToHash("0xa"),
				Miner:        types.StringToAddress("0xb").Bytes(),
				StateRoot:    types.StringToHash("0xc"),
				TxRoot:       types.StringToHash("0xd"),
				ReceiptsRoot: types.StringToHash("0xe"),
				Difficulty:   0,
				Number:       2,
				GasLimit:     200000,
				GasUsed:      140000,
				Timestamp:    1626362400,
				ExtraData:    []byte{0x4, 0x5, 0x6},
				MixHash:      types.StringToHash("0xf"),
				Nonce:        types.Nonce{2},
				BaseFee:      200,
			},
		}

		expectedRoot := types.StringToHash("0xb9d8212eaada25773fe552dd51b9b4ee6a77e6105b1841b556a978cd3bed5468")

		root := CalculateUncleRoot(uncles)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})

	t.Run("no uncles", func(t *testing.T) {
		t.Parallel()

		uncles := []*types.Header{}

		expectedRoot := types.EmptyUncleHash

		root := CalculateUncleRoot(uncles)

		assert.Equal(t, expectedRoot, root, "Unexpected root value")
	})
}
