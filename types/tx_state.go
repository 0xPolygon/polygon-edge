package types

import "math/big"

// StateTx implements Transaction interface with the state tx logic
type StateTx struct {
	baseTx
}

func NewEmptyStateTx() *StateTx {
	return &StateTx{
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
func (t *StateTx) Copy() TxData {
	return &StateTx{
		baseTx: *t.baseTx.copy(),
	}
}

func (t *StateTx) txType() TxType { return StateTxType }
