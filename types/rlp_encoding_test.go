package types

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

type codec interface {
	RLPMarshaler
	RLPUnmarshaler
}

func TestRLPEncoding(t *testing.T) {
	cases := []codec{
		&Header{},
		&Receipt{},
	}
	for _, c := range cases {
		buf := c.MarshalRLPTo(nil)

		res, ok := reflect.New(reflect.TypeOf(c).Elem()).Interface().(codec)
		if !ok {
			t.Fatalf("Unable to assert type")
		}

		if err := res.UnmarshalRLP(buf); err != nil {
			t.Fatal(err)
		}

		buf2 := c.MarshalRLPTo(nil)
		if !reflect.DeepEqual(buf, buf2) {
			t.Fatal("[ERROR] Buffers not equal")
		}
	}
}

func TestRLPMarshall_And_Unmarshall_Transaction(t *testing.T) {
	addrTo := StringToAddress("11")
	txn := &Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(11),
		Gas:      11,
		To:       &addrTo,
		Value:    big.NewInt(1),
		Input:    []byte{1, 2},
		V:        big.NewInt(25),
		S:        big.NewInt(26),
		R:        big.NewInt(27),
	}
	unmarshalledTxn := new(Transaction)
	marshaledRlp := txn.MarshalRLP()

	if err := unmarshalledTxn.UnmarshalRLP(marshaledRlp); err != nil {
		t.Fatal(err)
	}

	unmarshalledTxn.ComputeHash()

	txn.Hash = unmarshalledTxn.Hash
	if !reflect.DeepEqual(txn, unmarshalledTxn) {
		t.Fatal("[ERROR] Unmarshalled transaction not equal to base transaction")
	}
}

func TestRLPStorage_Marshall_And_Unmarshall_Receipt(t *testing.T) {
	addr := StringToAddress("11")
	hash := StringToHash("10")

	testTable := []struct {
		name      string
		receipt   *Receipt
		setStatus bool
	}{
		{
			"Marshal receipt with status",
			&Receipt{
				CumulativeGasUsed: 10,
				GasUsed:           100,
				ContractAddress:   &addr,
				TxHash:            hash,
			},
			true,
		},
		{
			"Marshal receipt without status",
			&Receipt{
				Root:              hash,
				CumulativeGasUsed: 10,
				GasUsed:           100,
				ContractAddress:   &addr,
				TxHash:            hash,
			},
			false,
		},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			receipt := testCase.receipt

			if testCase.setStatus {
				receipt.SetStatus(ReceiptSuccess)
			}

			unmarshalledReceipt := new(Receipt)
			marshaledRlp := receipt.MarshalStoreRLPTo(nil)

			if err := unmarshalledReceipt.UnmarshalStoreRLP(marshaledRlp); err != nil {
				t.Fatal(err)
			}

			if !assert.Exactly(t, receipt, unmarshalledReceipt) {
				t.Fatal("[ERROR] Unmarshalled receipt not equal to base receipt")
			}
		})
	}
}

func TestRLPUnmarshal_Header_ComputeHash(t *testing.T) {
	// header computes hash after unmarshalling
	h := &Header{}
	h.ComputeHash()

	data := h.MarshalRLP()
	h2 := new(Header)
	assert.NoError(t, h2.UnmarshalRLP(data))
	assert.Equal(t, h.Hash, h2.Hash)
}
