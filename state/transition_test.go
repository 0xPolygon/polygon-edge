package state

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func newTestTransition(preState map[types.Address]*PreState) *Transition {
	if preState == nil {
		preState = defaultPreState
	}
	return &Transition{
		logger: hclog.NewNullLogger(),
		state:  newTestTxn(preState),
	}
}

func TestSubGasLimitPrice(t *testing.T) {
	tests := []struct {
		name        string
		preState    map[types.Address]*PreState
		from        types.Address
		gas         uint64
		gasPrice    int64
		expectedErr error
	}{
		{
			name: "should succeed and reduce cost for maximum gas from account balance",
			preState: map[types.Address]*PreState{
				addr1: {
					Nonce:   0,
					Balance: 1000,
					State:   map[types.Hash]types.Hash{},
				},
			},
			from:        addr1,
			gas:         10,
			gasPrice:    10,
			expectedErr: nil,
		},
		{
			name: "should fail by ErrNotEnoughFunds",
			preState: map[types.Address]*PreState{
				addr1: {
					Nonce:   0,
					Balance: 10,
					State:   map[types.Hash]types.Hash{},
				},
			},
			from:     addr1,
			gas:      10,
			gasPrice: 10,
			// should return ErrNotEnoughFundsForGas when state.SubBalance returns ErrNotEnoughFunds
			expectedErr: ErrNotEnoughFundsForGas,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transition := newTestTransition(tt.preState)
			msg := &types.Transaction{
				From:     tt.from,
				Gas:      tt.gas,
				GasPrice: big.NewInt(tt.gasPrice),
			}

			err := transition.subGasLimitPrice(msg)

			assert.Equal(t, tt.expectedErr, err)
			if err == nil {
				// should reduce cost for gas from balance
				reducedAmount := new(big.Int).Mul(msg.GasPrice, big.NewInt(int64(msg.Gas)))
				newBalance := transition.GetBalance(msg.From)
				diff := new(big.Int).Sub(big.NewInt(int64(tt.preState[msg.From].Balance)), newBalance)
				assert.Zero(t, diff.Cmp(reducedAmount))
			}
		})
	}
}
