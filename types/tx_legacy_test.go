package types

import (
	"math/big"
	"reflect"
	"testing"
)

func TestLegacyTransactionCopy(t *testing.T) {
	addrTo := StringToAddress("11")
	txn := &LegacyTx{
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

	newTxn := txn.Copy()

	if !reflect.DeepEqual(txn, newTxn) {
		t.Fatal("[ERROR] Copied transaction not equal base transaction")
	}
}
