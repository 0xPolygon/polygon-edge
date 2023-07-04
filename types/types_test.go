package types

import (
	"math/big"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEIP55(t *testing.T) {
	t.Parallel()

	cases := []struct {
		address  string
		expected string
	}{
		{
			"0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed",
			"0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed",
		},
		{
			"0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359",
			"0xfB6916095ca1df60bB79Ce92cE3Ea74c37c5d359",
		},
		{
			"0xdbf03b407c01e7cd3cbea99509d93f8dddc8c6fb",
			"0xdbF03B407c01E7cD3CBea99509d93f8DDDC8C6FB",
		},
		{
			"0xd1220a0cf47c7b9be7a2e6ba89f429762e7b9adb",
			"0xD1220A0cf47c7B9Be7A2E6BA89F429762e7b9aDb",
		},
		{
			"0xde64a66c41599905950ca513fa432187a8c65679",
			"0xde64A66C41599905950ca513Fa432187a8C65679",
		},
		{
			"0xb41364957365228984ea8ee98e80dbed4b9ffcdc",
			"0xB41364957365228984eA8EE98e80DBED4B9fFcDC",
		},
		{
			"0xb529594951753de833b00865d7fe52cc4d8b0f63",
			"0xB529594951753DE833b00865D7FE52cC4d8B0f63",
		},
		{
			"0xb529594951753de833b00865",
			"0x0000000000000000B529594951753De833B00865",
		},
		{
			"0xeEd210D",
			"0x000000000000000000000000000000000eED210d",
		},
	}

	for _, c := range cases {
		c := c

		t.Run("", func(t *testing.T) {
			t.Parallel()

			addr := StringToAddress(c.address)
			assert.Equal(t, c.expected, addr.String())
		})
	}
}

func TestTransactionCopy(t *testing.T) {
	addrTo := StringToAddress("11")
	txn := &Transaction{
		Nonce:     0,
		GasTipCap: big.NewInt(11),
		GasFeeCap: big.NewInt(11),
		GasPrice:  big.NewInt(11),
		Gas:       11,
		To:        &addrTo,
		Value:     big.NewInt(1),
		Input:     []byte{1, 2},
		V:         big.NewInt(25),
		S:         big.NewInt(26),
		R:         big.NewInt(27),
	}
	newTxn := txn.Copy()

	if !reflect.DeepEqual(txn, newTxn) {
		t.Fatal("[ERROR] Copied transaction not equal base transaction")
	}
}

func TestIsValidAddress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		address string
		isValid bool
	}{
		{address: "0x123", isValid: false},
		{address: "FooBar", isValid: false},
		{address: "123FooBar", isValid: false},
		{address: "0x1234567890987654321012345678909876543210", isValid: true},
		{address: "0x0000000000000000000000000000000000000000", isValid: true},
		{address: "0x1000000000000000000000000000000000000000", isValid: true},
	}

	for _, c := range cases {
		err := IsValidAddress(c.address)
		if c.isValid {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
