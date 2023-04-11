package state

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/require"
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
