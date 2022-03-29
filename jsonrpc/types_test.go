package jsonrpc

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/0xPolygon/polygon-edge/helper/hex"

	"github.com/0xPolygon/polygon-edge/types"
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
