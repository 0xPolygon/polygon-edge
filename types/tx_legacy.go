package types

// LegacyTx implements Transaction interface with the legacy tx logic
type LegacyTx struct {
	baseTx
}

// Copy creates a deep copy of the transaction data and initializes all fields.
func (t *LegacyTx) Copy() TxData {
	return &LegacyTx{
		baseTx: *t.baseTx.copy(),
	}
}

func (t *LegacyTx) txType() TxType { return LegacyTxType }
