package jsonrpc

import (
	"bytes"
	"encoding/json"
	"github.com/0xPolygon/polygon-sdk/helper/hex"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/0xPolygon/polygon-sdk/types"
	"github.com/stretchr/testify/assert"
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
				"to": "{{.Addr}}",
				"gas": "0x10",
				"input": "0x01",
				"value": "0x01"
			}`,
			res: &txnArgs{
				To:    &addr,
				Gas:   &num,
				Input: &hex,
				Value: &hex,
			},
		},
	}

	for _, c := range cases {
		tmpl, err := template.New("test").Parse(c.data)
		assert.NoError(t, err)

		config := map[string]string{
			"Addr": (types.Address{}).String(),
			"Hash": (types.Hash{}).String(),
		}

		buffer := new(bytes.Buffer)
		assert.NoError(t, tmpl.Execute(buffer, config))

		r := &txnArgs{}
		assert.NoError(t, json.Unmarshal(buffer.Bytes(), &r))

		if !reflect.DeepEqual(r, c.res) {
			t.Fatal("bad")
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
		V:        v,
		R:        r,
		S:        s,
		Hash:     types.Hash{},
		From:     types.Address{},
	}
	block := types.Block{
		Transactions: []*types.Transaction{&txn},
		Header:       &types.Header{},
	}

	jsonTx := toTransaction(&txn, &block, 0)

	jsonV, _ := jsonTx.V.MarshalText()
	jsonR, _ := jsonTx.V.MarshalText()
	jsonS, _ := jsonTx.V.MarshalText()
	assert.Equal(t, hexWithoutLeading0, string(jsonV))
	assert.Equal(t, hexWithoutLeading0, string(jsonR))
	assert.Equal(t, hexWithoutLeading0, string(jsonS))
}