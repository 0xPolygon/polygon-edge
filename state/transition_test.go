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
		msg         *types.Transaction
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
			msg: &types.Transaction{
				From:     addr1,
				Gas:      10,
				GasPrice: big.NewInt(10),
			},
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
			msg: &types.Transaction{
				From:     addr1,
				Gas:      10,
				GasPrice: big.NewInt(10),
			},
			// should return ErrNotEnoughFundsForGas when state.SubBalance returns ErrNotEnoughFunds
			expectedErr: ErrNotEnoughFundsForGas,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transition := newTestTransition(tt.preState)
			err := transition.subGasLimitPrice(tt.msg)
			assert.Equal(t, tt.expectedErr, err)
		})
	}
}
