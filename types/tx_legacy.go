package types

import "math/big"

// LegacyTx implements Transaction interface with the legacy tx logic
type LegacyTx struct {
	baseTx
}

func NewEmptyLegacyTx() *LegacyTx {
	return &LegacyTx{
		baseTx: baseTx{
			GasPrice: new(big.Int),
			Value:    new(big.Int),
			V:        new(big.Int),
			R:        new(big.Int),
			S:        new(big.Int),
		},
	}
}

// Copy creates a deep copy of the transaction data and initializes all fields.
func (t *LegacyTx) Copy() TxData {
	return &LegacyTx{
		baseTx: *t.baseTx.copy(),
	}
}

func (t *LegacyTx) txType() TxType { return LegacyTxType }
