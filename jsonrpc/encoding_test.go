package jsonrpc

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
	"testing"
	"text/template"

	"github.com/0xPolygon/minimal/types"
	"github.com/stretchr/testify/assert"
)

func TestDecode_Types(t *testing.T) {
	// decode basic types
	cases := []struct {
		obj interface{}
		res string
	}{
		{
			argBigPtr(big.NewInt(10)),
			"0xa",
		},
		{
			argUint64(10),
			"0xa",
		},
		{
			argBytesPtr([]byte{0x1, 0x2}),
			"0x0102",
		},
	}

	for _, c := range cases {
		res, err := json.Marshal(c.obj)
		assert.NoError(t, err)
		assert.Equal(t, strings.Trim(string(res), "\""), c.res)
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
