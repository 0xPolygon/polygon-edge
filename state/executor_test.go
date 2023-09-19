package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
)

func TestOverride(t *testing.T) {
	t.Parallel()

	state := newStateWithPreState(map[types.Address]*PreState{
		{0x0}: {
			Nonce:   1,
			Balance: 1,
			State: map[types.Hash]types.Hash{
				types.ZeroHash: {0x1},
			},
		},
		{0x1}: {
			State: map[types.Hash]types.Hash{
				types.ZeroHash: {0x1},
			},
		},
	})

	nonce := uint64(2)
	balance := big.NewInt(2)
	code := []byte{0x1}

	tt := NewTransition(chain.ForksInTime{}, state, newTxn(state))

	require.Empty(t, tt.state.GetCode(types.ZeroAddress))

	err := tt.WithStateOverride(types.StateOverride{
		{0x0}: types.OverrideAccount{
			Nonce:   &nonce,
			Balance: balance,
			Code:    code,
			StateDiff: map[types.Hash]types.Hash{
				types.ZeroHash: {0x2},
			},
		},
		{0x1}: types.OverrideAccount{
			State: map[types.Hash]types.Hash{
				{0x1}: {0x1},
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, nonce, tt.state.GetNonce(types.ZeroAddress))
	require.Equal(t, balance, tt.state.GetBalance(types.ZeroAddress))
	require.Equal(t, code, tt.state.GetCode(types.ZeroAddress))
	require.Equal(t, types.Hash{0x2}, tt.state.GetState(types.ZeroAddress, types.ZeroHash))

	// state is fully replaced
	require.Equal(t, types.Hash{0x0}, tt.state.GetState(types.Address{0x1}, types.ZeroHash))
	require.Equal(t, types.Hash{0x1}, tt.state.GetState(types.Address{0x1}, types.Hash{0x1}))
}

func Test_Transition_checkDynamicFees(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseFee *big.Int
		tx      *types.Transaction
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "happy path",
			baseFee: big.NewInt(100),
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasFeeCap: big.NewInt(100),
				GasTipCap: big.NewInt(100),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				assert.NoError(t, err, i)

				return false
			},
		},
		{
			name:    "happy path with empty values",
			baseFee: big.NewInt(0),
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasFeeCap: big.NewInt(0),
				GasTipCap: big.NewInt(0),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				assert.NoError(t, err, i)

				return false
			},
		},
		{
			name:    "gas fee cap less than base fee",
			baseFee: big.NewInt(20),
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasFeeCap: big.NewInt(10),
				GasTipCap: big.NewInt(0),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				expectedError := fmt.Sprintf("max fee per gas less than block base fee: "+
					"address %s, GasFeeCap/GasPrice: 10, BaseFee: 20", types.ZeroAddress)
				assert.EqualError(t, err, expectedError, i)

				return true
			},
		},
		{
			name:    "gas fee cap less than tip cap",
			baseFee: big.NewInt(5),
			tx: &types.Transaction{
				Type:      types.DynamicFeeTx,
				GasFeeCap: big.NewInt(10),
				GasTipCap: big.NewInt(15),
			},
			wantErr: func(t assert.TestingT, err error, i ...interface{}) bool {
				expectedError := fmt.Sprintf("max priority fee per gas higher than max fee per gas: "+
					"address %s, GasTipCap: 15, GasFeeCap: 10", types.ZeroAddress)
				assert.EqualError(t, err, expectedError, i)

				return true
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := &Transition{
				ctx: runtime.TxContext{
					BaseFee: tt.baseFee,
				},
				config: chain.ForksInTime{
					London: true,
				},
			}

			err := tr.checkDynamicFees(tt.tx)
			tt.wantErr(t, err, fmt.Sprintf("checkDynamicFees(%v)", tt.tx))
		})
	}
}
