package types

// StateTx implements Transaction interface with the state tx logic
type StateTx struct {
	baseTx
}

// Copy creates a deep copy of the transaction data and initializes all fields.
func (t *StateTx) Copy() TxData {
	return &StateTx{
		baseTx: *t.baseTx.copy(),
	}
}

func (t *StateTx) txType() TxType { return StateTxType }
