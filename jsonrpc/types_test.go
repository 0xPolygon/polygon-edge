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
	txn := types.Transaction{
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

	jsonTx := toTransaction(&txn, nil, nil, nil)

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
	b := block{
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
	}

	testBlock := func(name string) {
		res, err := json.Marshal(b)
		require.NoError(t, err)

		data, err := testsuite.ReadFile(name)
		require.NoError(t, err)

		data = removeWhiteSpace(data)
		require.Equal(t, res, data)
	}

	t.Run("empty block", func(t *testing.T) {
		testBlock("testsuite/block-empty.json")
	})

	t.Run("block with transaction hashes", func(t *testing.T) {
		b.Transactions = []transactionOrHash{
			transactionHash{0x8},
		}
		testBlock("testsuite/block-with-txn-hashes.json")
	})

	t.Run("block with transaction bodies", func(t *testing.T) {
		b.Transactions = []transactionOrHash{
			mockTxn(),
		}
		testBlock("testsuite/block-with-txn-bodies.json")
	})
}

func removeWhiteSpace(d []byte) []byte {
	s := string(d)
	s = strings.Replace(s, "\n", "", -1)
	s = strings.Replace(s, "\t", "", -1)
	s = strings.Replace(s, " ", "", -1)

	return []byte(s)
}

func mockTxn() *transaction {
	to := types.Address{}

	tt := &transaction{
		Nonce:       1,
		GasPrice:    argBig(*big.NewInt(10)),
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
	}

	return tt
}

func TestTransaction_Encoding(t *testing.T) {
	tt := mockTxn()

	testTransaction := func(name string) {
		res, err := json.Marshal(tt)
		require.NoError(t, err)

		data, err := testsuite.ReadFile(name)
		require.NoError(t, err)

		data = removeWhiteSpace(data)
		require.Equal(t, res, data)
	}

	t.Run("sealed", func(t *testing.T) {
		testTransaction("testsuite/transaction-sealed.json")
	})

	t.Run("pending", func(t *testing.T) {
		tt.BlockHash = nil
		tt.BlockNumber = nil
		tt.TxIndex = nil

		testTransaction("testsuite/transaction-pending.json")
	})
}
