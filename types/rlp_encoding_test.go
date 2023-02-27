package types

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/umbracle/fastrlp"

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
	assert.Equal(t, txn, unmarshalledTxn, "[ERROR] Unmarshalled transaction not equal to base transaction")
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

func TestRLPMarshall_And_Unmarshall_TypedTransaction(t *testing.T) {
	addrTo := StringToAddress("11")
	addrFrom := StringToAddress("22")
	originalTx := &Transaction{
		Nonce:    0,
		GasPrice: big.NewInt(11),
		Gas:      11,
		To:       &addrTo,
		From:     addrFrom,
		Value:    big.NewInt(1),
		Input:    []byte{1, 2},
		V:        big.NewInt(25),
		S:        big.NewInt(26),
		R:        big.NewInt(27),
	}

	txTypes := []TxType{
		StateTx,
		LegacyTx,
	}

	for _, v := range txTypes {
		originalTx.Type = v
		originalTx.ComputeHash()

		txRLP := originalTx.MarshalRLP()

		unmarshalledTx := new(Transaction)
		assert.NoError(t, unmarshalledTx.UnmarshalRLP(txRLP))

		unmarshalledTx.ComputeHash()
		assert.Equal(t, originalTx.Type, unmarshalledTx.Type)
		assert.Equal(t, originalTx.Hash, unmarshalledTx.Hash)
	}
}

func TestRLPMarshall_Unmarshall_Missing_Data(t *testing.T) {
	t.Parallel()

	txTypes := []TxType{
		StateTx,
		LegacyTx,
	}

	for _, txType := range txTypes {
		txType := txType
		testTable := []struct {
			name          string
			expectedErr   bool
			ommitedValues map[string]bool
			fromAddrSet   bool
		}{
			{
				name:        "Insuficient params",
				expectedErr: true,
				ommitedValues: map[string]bool{
					"Nonce":    true,
					"GasPrice": true,
				},
			},
			{
				name:        "Missing From",
				expectedErr: false,
				ommitedValues: map[string]bool{
					"From": true,
				},
				fromAddrSet: false,
			},
			{
				name:          "Address set for state tx only",
				expectedErr:   false,
				ommitedValues: map[string]bool{},
				fromAddrSet:   txType == StateTx,
			},
		}

		for _, tt := range testTable {
			tt := tt
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				arena := fastrlp.DefaultArenaPool.Get()
				parser := fastrlp.DefaultParserPool.Get()
				testData := testRLPData(arena, tt.ommitedValues)
				v, err := parser.Parse(testData)
				assert.Nil(t, err)

				unmarshalledTx := &Transaction{Type: txType}

				if tt.expectedErr {
					assert.Error(t, unmarshalledTx.unmarshalRLPFrom(parser, v), tt.name)
				} else {
					assert.NoError(t, unmarshalledTx.unmarshalRLPFrom(parser, v), tt.name)
					assert.Equal(t, tt.fromAddrSet, len(unmarshalledTx.From) != 0 && unmarshalledTx.From != ZeroAddress, unmarshalledTx.Type.String(), unmarshalledTx.From)
				}

				fastrlp.DefaultParserPool.Put(parser)
				fastrlp.DefaultArenaPool.Put(arena)
			})
		}
	}
}

func TestRLPMarshall_And_Unmarshall_TxType(t *testing.T) {
	testTable := []struct {
		name        string
		txType      TxType
		expectedErr bool
	}{
		{
			name:   "StateTx",
			txType: StateTx,
		},
		{
			name:   "LegacyTx",
			txType: LegacyTx,
		},
		{
			name:        "undefined type",
			txType:      TxType(0x09),
			expectedErr: true,
		},
	}

	for _, tt := range testTable {
		ar := &fastrlp.Arena{}

		var txType TxType
		err := txType.unmarshalRLPFrom(nil, ar.NewBytes([]byte{byte(tt.txType)}))

		if tt.expectedErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
			assert.Equal(t, tt.txType, txType)
		}
	}
}

func testRLPData(arena *fastrlp.Arena, ommitValues map[string]bool) []byte {
	vv := arena.NewArray()

	_, ommit := ommitValues["Nonce"]
	if !ommit {
		vv.Set(arena.NewUint(10))
	}

	_, ommit = ommitValues["GasPrice"]
	if !ommit {
		vv.Set(arena.NewBigInt(big.NewInt(11)))
	}

	_, ommit = ommitValues["Gas"]
	if !ommit {
		vv.Set(arena.NewUint(12))
	}

	_, ommit = ommitValues["To"]
	if !ommit {
		vv.Set(arena.NewBytes((StringToAddress("13")).Bytes()))
	}

	_, ommit = ommitValues["Value"]
	if !ommit {
		vv.Set(arena.NewBigInt(big.NewInt(14)))
	}

	_, ommit = ommitValues["Input"]
	if !ommit {
		vv.Set(arena.NewCopyBytes([]byte{1, 2}))
	}

	_, ommit = ommitValues["V"]
	if !ommit {
		vv.Set(arena.NewBigInt(big.NewInt(15)))
	}

	_, ommit = ommitValues["R"]
	if !ommit {
		vv.Set(arena.NewBigInt(big.NewInt(16)))
	}

	_, ommit = ommitValues["S"]
	if !ommit {
		vv.Set(arena.NewBigInt(big.NewInt(17)))
	}

	_, ommit = ommitValues["From"]
	if !ommit {
		vv.Set(arena.NewBytes((StringToAddress("18")).Bytes()))
	}

	var testData []byte
	testData = vv.MarshalTo(testData)

	return testData
}
