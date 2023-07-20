package types

import (
	"fmt"
	"math/big"
	"reflect"
	"testing"

	"github.com/umbracle/fastrlp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	txn.ComputeHash(1)

	unmarshalledTxn := new(Transaction)
	marshaledRlp := txn.MarshalRLP()

	if err := unmarshalledTxn.UnmarshalRLP(marshaledRlp); err != nil {
		t.Fatal(err)
	}

	unmarshalledTxn.ComputeHash(1)

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
		Nonce:     0,
		GasPrice:  big.NewInt(11),
		GasFeeCap: big.NewInt(12),
		GasTipCap: big.NewInt(13),
		Gas:       11,
		To:        &addrTo,
		From:      addrFrom,
		Value:     big.NewInt(1),
		Input:     []byte{1, 2},
		V:         big.NewInt(25),
		S:         big.NewInt(26),
		R:         big.NewInt(27),
	}

	txTypes := []TxType{
		StateTx,
		LegacyTx,
		DynamicFeeTx,
	}

	for _, v := range txTypes {
		t.Run(v.String(), func(t *testing.T) {
			originalTx.Type = v
			originalTx.ComputeHash(1)

			txRLP := originalTx.MarshalRLP()

			unmarshalledTx := new(Transaction)
			assert.NoError(t, unmarshalledTx.UnmarshalRLP(txRLP))

			unmarshalledTx.ComputeHash(1)
			assert.Equal(t, originalTx.Type, unmarshalledTx.Type)
			assert.Equal(t, originalTx.Hash, unmarshalledTx.Hash)
		})
	}
}

func TestRLPMarshall_Unmarshall_Missing_Data(t *testing.T) {
	t.Parallel()

	txTypes := []TxType{
		StateTx,
		LegacyTx,
		DynamicFeeTx,
	}

	for _, txType := range txTypes {
		txType := txType
		testTable := []struct {
			name          string
			expectedErr   bool
			omittedValues map[string]bool
			fromAddrSet   bool
		}{
			{
				name:        fmt.Sprintf("[%s] Insufficient params", txType),
				expectedErr: true,
				omittedValues: map[string]bool{
					"Nonce":    true,
					"GasPrice": true,
				},
			},
			{
				name:        fmt.Sprintf("[%s] Missing From", txType),
				expectedErr: false,
				omittedValues: map[string]bool{
					"ChainID":    txType != DynamicFeeTx,
					"GasTipCap":  txType != DynamicFeeTx,
					"GasFeeCap":  txType != DynamicFeeTx,
					"GasPrice":   txType == DynamicFeeTx,
					"AccessList": txType != DynamicFeeTx,
					"From":       txType != StateTx,
				},
				fromAddrSet: txType == StateTx,
			},
			{
				name:        fmt.Sprintf("[%s] Address set for state tx only", txType),
				expectedErr: false,
				omittedValues: map[string]bool{
					"ChainID":    txType != DynamicFeeTx,
					"GasTipCap":  txType != DynamicFeeTx,
					"GasFeeCap":  txType != DynamicFeeTx,
					"GasPrice":   txType == DynamicFeeTx,
					"AccessList": txType != DynamicFeeTx,
					"From":       txType != StateTx,
				},
				fromAddrSet: txType == StateTx,
			},
		}

		for _, tt := range testTable {
			tt := tt

			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				arena := fastrlp.DefaultArenaPool.Get()
				parser := fastrlp.DefaultParserPool.Get()
				testData := testRLPData(arena, tt.omittedValues)
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
			name:   "DynamicFeeTx",
			txType: DynamicFeeTx,
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

func testRLPData(arena *fastrlp.Arena, omitValues map[string]bool) []byte {
	vv := arena.NewArray()

	if omit, _ := omitValues["ChainID"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(0)))
	}

	if omit, _ := omitValues["Nonce"]; !omit {
		vv.Set(arena.NewUint(10))
	}

	if omit, _ := omitValues["GasTipCap"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(11)))
	}

	if omit, _ := omitValues["GasFeeCap"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(11)))
	}

	if omit, _ := omitValues["GasPrice"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(11)))
	}

	if omit, _ := omitValues["Gas"]; !omit {
		vv.Set(arena.NewUint(12))
	}

	if omit, _ := omitValues["To"]; !omit {
		vv.Set(arena.NewBytes((StringToAddress("13")).Bytes()))
	}

	if omit, _ := omitValues["Value"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(14)))
	}

	if omit, _ := omitValues["Input"]; !omit {
		vv.Set(arena.NewCopyBytes([]byte{1, 2}))
	}

	if omit, _ := omitValues["AccessList"]; !omit {
		vv.Set(arena.NewArray())
	}

	if omit, _ := omitValues["V"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(15)))
	}

	if omit, _ := omitValues["R"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(16)))
	}

	if omit, _ := omitValues["S"]; !omit {
		vv.Set(arena.NewBigInt(big.NewInt(17)))
	}

	if omit, _ := omitValues["From"]; !omit {
		vv.Set(arena.NewBytes((StringToAddress("18")).Bytes()))
	}

	var testData []byte
	testData = vv.MarshalTo(testData)

	return testData
}

func Test_MarshalCorruptedBytesArray(t *testing.T) {
	t.Parallel()

	marshal := func(obj func(*fastrlp.Arena) *fastrlp.Value) []byte {
		ar := fastrlp.DefaultArenaPool.Get()
		defer fastrlp.DefaultArenaPool.Put(ar)

		return obj(ar).MarshalTo(nil)
	}

	emptyArray := [8]byte{}
	corruptedSlice := make([]byte, 32)
	corruptedSlice[29], corruptedSlice[30], corruptedSlice[31] = 5, 126, 64
	intOfCorruption := uint64(18_446_744_073_709_551_615) // 2^64-1

	marshalOne := func(ar *fastrlp.Arena) *fastrlp.Value {
		return ar.NewBytes(corruptedSlice)
	}

	marshalTwo := func(ar *fastrlp.Arena) *fastrlp.Value {
		return ar.NewUint(intOfCorruption)
	}

	marshal(marshalOne)

	require.Equal(t, emptyArray[:], corruptedSlice[:len(emptyArray)])

	marshal(marshalTwo) // without fixing this, marshaling will cause corruption of the corrupted slice

	require.Equal(t, emptyArray[:], corruptedSlice[:len(emptyArray)])
}
