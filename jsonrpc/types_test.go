package jsonrpc

import (
	"bytes"
	"embed"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/0xPolygon/polygon-edge/helper/hex"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicTypes_Encode(t *testing.T) {
	// decode basic types
	cases := []struct {
		obj interface{}
		dec interface{}
		res string
	}{
		{
			argBig(*big.NewInt(10)),
			&argBig{},
			"0xa",
		},
		{
			argUint64(10),
			argUintPtr(0),
			"0xa",
		},
		{
			argBytes([]byte{0x1, 0x2}),
			&argBytes{},
			"0x0102",
		},
	}

	for _, c := range cases {
		res, err := json.Marshal(c.obj)
		assert.NoError(t, err)
		assert.Equal(t, strings.Trim(string(res), "\""), c.res)
		assert.NoError(t, json.Unmarshal(res, c.dec))
	}
}

func TestDecode_TxnArgs(t *testing.T) {
	var (
		addr = types.Address{0x0}
		num  = argUint64(16)
		hex  = argBytes([]byte{0x01})
	)

	cases := []struct {
		data string
		res  *txnArgs
	}{
		{
			data: `{
				"to": "{{.Libp2pAddr}}",
				"gas": "0x10",
				"data": "0x01",
				"value": "0x01"
			}`,
			res: &txnArgs{
				To:    &addr,
				Gas:   &num,
				Data:  &hex,
				Value: &hex,
			},
		},
	}

	for _, c := range cases {
		tmpl, err := template.New("test").Parse(c.data)
		assert.NoError(t, err)

		config := map[string]string{
			"Libp2pAddr": (types.Address{}).String(),
			"Hash":       (types.Hash{}).String(),
		}

		buffer := new(bytes.Buffer)
		assert.NoError(t, tmpl.Execute(buffer, config))

		r := &txnArgs{}
		assert.NoError(t, json.Unmarshal(buffer.Bytes(), &r))

		if !reflect.DeepEqual(r, c.res) {
			t.Fatal("Resulting data and expected values are not equal")
		}
	}
}

func TestToTransaction_Returns_V_R_S_ValuesWithoutLeading0(t *testing.T) {
	hexWithLeading0 := "0x0ba93811466694b3b3cb8853cb8227b7c9f49db10bf6e7db59d20ac904961565"
	hexWithoutLeading0 := "0xba93811466694b3b3cb8853cb8227b7c9f49db10bf6e7db59d20ac904961565"
	v, _ := hex.DecodeHex(hexWithLeading0)
	r, _ := hex.DecodeHex(hexWithLeading0)
	s, _ := hex.DecodeHex(hexWithLeading0)
	txn := &types.Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(0),
		Gas:      0,
		To:       nil,
		Value:    big.NewInt(0),
		Input:    nil,
		V:        new(big.Int).SetBytes(v),
		R:        new(big.Int).SetBytes(r),
		S:        new(big.Int).SetBytes(s),
		Hash:     types.Hash{},
		From:     types.Address{},
	}

	jsonTx := toTransaction(txn, nil, nil, nil)

	jsonV, _ := jsonTx.V.MarshalText()
	jsonR, _ := jsonTx.R.MarshalText()
	jsonS, _ := jsonTx.S.MarshalText()

	assert.Equal(t, hexWithoutLeading0, string(jsonV))
	assert.Equal(t, hexWithoutLeading0, string(jsonR))
	assert.Equal(t, hexWithoutLeading0, string(jsonS))
}

func TestToTransaction_EIP1559(t *testing.T) {
	hexWithLeading0 := "0x0ba93811466694b3b3cb8853cb8227b7c9f49db10bf6e7db59d20ac904961565"
	hexWithoutLeading0 := "0xba93811466694b3b3cb8853cb8227b7c9f49db10bf6e7db59d20ac904961565"
	v, _ := hex.DecodeHex(hexWithLeading0)
	r, _ := hex.DecodeHex(hexWithLeading0)
	s, _ := hex.DecodeHex(hexWithLeading0)
	txn := &types.Transaction{
		Nonce:     0,
		GasPrice:  nil,
		GasTipCap: big.NewInt(10),
		GasFeeCap: big.NewInt(10),
		Gas:       0,
		To:        nil,
		Value:     big.NewInt(0),
		Input:     nil,
		V:         new(big.Int).SetBytes(v),
		R:         new(big.Int).SetBytes(r),
		S:         new(big.Int).SetBytes(s),
		Hash:      types.Hash{},
		From:      types.Address{},
	}

	jsonTx := toTransaction(txn, nil, nil, nil)

	jsonV, _ := jsonTx.V.MarshalText()
	jsonR, _ := jsonTx.R.MarshalText()
	jsonS, _ := jsonTx.S.MarshalText()

	assert.Equal(t, hexWithoutLeading0, string(jsonV))
	assert.Equal(t, hexWithoutLeading0, string(jsonR))
	assert.Equal(t, hexWithoutLeading0, string(jsonS))
}

func TestBlock_Copy(t *testing.T) {
	b := &block{
		ExtraData: []byte{0x1},
		Miner:     []byte{0x2},
		Uncles:    []types.Hash{{0x0, 0x1}},
	}

	bb := b.Copy()
	require.Equal(t, b, bb)
}

//go:embed testsuite/*
var testsuite embed.FS

func TestBlock_Encoding(t *testing.T) {
	getBlock := func() block {
		return block{
			ParentHash:   types.Hash{0x1},
			Sha3Uncles:   types.Hash{0x2},
			Miner:        types.Address{0x1}.Bytes(),
			StateRoot:    types.Hash{0x4},
			TxRoot:       types.Hash{0x5},
			ReceiptsRoot: types.Hash{0x6},
			LogsBloom:    types.Bloom{0x0},
			Difficulty:   10,
			Number:       11,
			GasLimit:     12,
			GasUsed:      13,
			Timestamp:    14,
			ExtraData:    []byte{97, 98, 99, 100, 101, 102},
			MixHash:      types.Hash{0x7},
			Nonce:        types.Nonce{10},
			Hash:         types.Hash{0x8},
			BaseFee:      15,
		}
	}

	testBlock := func(name string, b block) {
		res, err := json.Marshal(b)
		require.NoError(t, err)

		expectedData := loadTestData(t, name)
		require.JSONEq(t, expectedData, string(res))
	}

	t.Run("empty block", func(t *testing.T) {
		testBlock("testsuite/block-empty.json", getBlock())
	})

	t.Run("block with no base fee", func(t *testing.T) {
		b := getBlock()
		b.BaseFee = 0
		testBlock("testsuite/block-with-no-basefee.json", b)
	})

	t.Run("block with transaction hashes", func(t *testing.T) {
		b := getBlock()
		b.Transactions = []transactionOrHash{
			transactionHash{0x8},
		}
		testBlock("testsuite/block-with-txn-hashes.json", b)
	})

	t.Run("block with transaction bodies", func(t *testing.T) {
		b := getBlock()
		b.Transactions = []transactionOrHash{
			mockTxn(),
		}
		testBlock("testsuite/block-with-txn-bodies.json", b)
	})
}

func mockTxn() *transaction {
	to := types.StringToAddress("0x4")

	tt := &transaction{
		Nonce:       1,
		GasPrice:    argBigPtr(big.NewInt(10)),
		Gas:         100,
		To:          &to,
		Value:       argBig(*big.NewInt(1000)),
		Input:       []byte{0x1, 0x2},
		V:           argBig(*big.NewInt(1)),
		R:           argBig(*big.NewInt(2)),
		S:           argBig(*big.NewInt(3)),
		Hash:        types.Hash{0x2},
		From:        types.Address{0x3},
		BlockHash:   &types.ZeroHash,
		BlockNumber: argUintPtr(1),
		TxIndex:     argUintPtr(2),
		Type:        argUint64(types.LegacyTx),
	}

	return tt
}

func TestTransaction_Encoding(t *testing.T) {
	testTransaction := func(name string, tt *transaction) {
		res, err := json.Marshal(tt)
		require.NoError(t, err)

		expectedData := loadTestData(t, name)
		require.JSONEq(t, expectedData, string(res))
	}

	t.Run("sealed", func(t *testing.T) {
		tt := mockTxn()

		testTransaction("testsuite/transaction-sealed.json", tt)
	})

	t.Run("pending", func(t *testing.T) {
		tt := mockTxn()
		tt.BlockHash = nil
		tt.BlockNumber = nil
		tt.TxIndex = nil

		testTransaction("testsuite/transaction-pending.json", tt)
	})

	t.Run("eip-1559", func(t *testing.T) {
		gasTipCap := argBig(*big.NewInt(10))
		gasFeeCap := argBig(*big.NewInt(10))

		tt := mockTxn()
		tt.GasTipCap = &gasTipCap
		tt.GasFeeCap = &gasFeeCap
		tt.Type = argUint64(types.DynamicFeeTx)

		testTransaction("testsuite/transaction-eip1559.json", tt)
	})
}

func Test_toReceipt(t *testing.T) {
	const (
		cumulativeGasUsed = 28000
		gasUsed           = 26000
	)

	testReceipt := func(name string, r *receipt) {
		res, err := json.Marshal(r)
		require.NoError(t, err)

		expectedData := loadTestData(t, name)
		require.JSONEq(t, expectedData, string(res))
	}

	t.Run("no logs", func(t *testing.T) {
		tx := createTestTransaction(types.StringToHash("tx1"))
		recipient := types.StringToAddress("2")
		tx.From = types.StringToAddress("1")
		tx.To = &recipient

		header := createTestHeader(15, nil)
		rec := createTestReceipt(nil, cumulativeGasUsed, gasUsed, tx.Hash)
		testReceipt("testsuite/receipt-no-logs.json", toReceipt(rec, tx, 0, header, nil))
	})

	t.Run("with contract address", func(t *testing.T) {
		tx := createTestTransaction(types.StringToHash("tx1"))
		tx.To = nil

		contractAddr := types.StringToAddress("3")
		header := createTestHeader(20, nil)
		rec := createTestReceipt(nil, cumulativeGasUsed, gasUsed, tx.Hash)
		rec.ContractAddress = &contractAddr
		testReceipt("testsuite/receipt-contract-deployment.json", toReceipt(rec, tx, 0, header, nil))
	})

	t.Run("with logs", func(t *testing.T) {
		tx := createTestTransaction(types.StringToHash("tx1"))
		recipient := types.StringToAddress("2")
		tx.From = types.StringToAddress("1")
		tx.To = &recipient

		header := createTestHeader(30, nil)
		logs := createTestLogs(2, recipient)
		originReceipt := createTestReceipt(logs, cumulativeGasUsed, gasUsed, tx.Hash)
		txIdx := uint64(1)
		receipt := toReceipt(originReceipt, tx, txIdx, header, toLogs(logs, 0, txIdx, header, tx.Hash))
		testReceipt("testsuite/receipt-with-logs.json", receipt)
	})
}

func Test_toBlock(t *testing.T) {
	h := createTestHeader(20, func(h *types.Header) {
		h.BaseFee = 200
		h.ParentHash = types.BytesToHash([]byte("ParentHash"))
	})

	block := &types.Block{
		Header: h,
		Transactions: []*types.Transaction{
			createTestTransaction(types.StringToHash("tx1")),
			createTestTransaction(types.StringToHash("tx2")),
		},
	}

	b := toBlock(block, true)
	require.NotNil(t, b)

	res, err := json.Marshal(b)
	require.NoError(t, err)

	expectedJSON := loadTestData(t, "testsuite/block-with-txn-full.json")
	require.JSONEq(t, expectedJSON, string(res))
}

func loadTestData(t *testing.T, name string) string {
	t.Helper()

	data, err := testsuite.ReadFile(name)
	require.NoError(t, err)

	return string(data)
}
