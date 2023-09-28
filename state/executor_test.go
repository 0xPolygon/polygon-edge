package state

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
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

// Tests for EIP-2929
func Test_Transition_EIP2929(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		code        []byte
		gasConsumed uint64
	}{
		{
			name: "Test 1: Check access list for EXTCODEHASH,EXTCODESIZE and BALANCE opcodes",
			code: []byte{
				// WarmStorageReadCostEIP2929 should be charged since precompiles address are in access list
				uint8(evm.PUSH1), 1, uint8(evm.EXTCODEHASH), uint8(evm.POP),
				uint8(evm.PUSH1), 2, uint8(evm.EXTCODESIZE), uint8(evm.POP),
				uint8(evm.PUSH1), 3, uint8(evm.BALANCE), uint8(evm.POP),

				uint8(evm.PUSH1), 0xe1, uint8(evm.EXTCODEHASH), uint8(evm.POP),
				uint8(evm.PUSH1), 0xe2, uint8(evm.EXTCODESIZE), uint8(evm.POP),
				uint8(evm.PUSH1), 0xe3, uint8(evm.BALANCE), uint8(evm.POP),
				// cost should be WarmStorageReadCostEIP2929, since addresses are present in access list
				uint8(evm.PUSH1), 0xe2, uint8(evm.EXTCODEHASH), uint8(evm.POP),
				uint8(evm.PUSH1), 0xe3, uint8(evm.EXTCODESIZE), uint8(evm.POP),
				uint8(evm.PUSH1), 0xe1, uint8(evm.BALANCE), uint8(evm.POP),

				uint8(evm.ORIGIN), uint8(evm.BALANCE), uint8(evm.POP),
				uint8(evm.ADDRESS), uint8(evm.BALANCE), uint8(evm.POP),

				uint8(evm.STOP),
			},
			gasConsumed: uint64(8653),
		},
		{
			name: "Test 2: Check Storage opcodes",
			code: []byte{
				// Add slot `0xe1` to access list, ColdAccountAccessCostEIP2929 charged
				uint8(evm.PUSH1), 0xe1, uint8(evm.SLOAD), uint8(evm.POP),
				// Write to `0xe1` which is already in access list, WarmStorageReadCostEIP2929 charged
				uint8(evm.PUSH1), 0xf1, uint8(evm.PUSH1), 0xe1, uint8(evm.SSTORE),
				// Write to `0xe2` which is not in access list, ColdAccountAccessCostEIP2929 charged
				uint8(evm.PUSH1), 0xf1, uint8(evm.PUSH1), 0xe2, uint8(evm.SSTORE),
				// Write again to `0xe2`, WarmStorageReadCostEIP2929 charged since `0xe2` already in access list
				uint8(evm.PUSH1), 0x11, uint8(evm.PUSH1), 0xe2, uint8(evm.SSTORE),
				// SLOAD `0xe2`, address present in access list
				uint8(evm.PUSH1), 0xe2, uint8(evm.SLOAD),
				// SLOAD `0xe3`, ColdStorageReadCostEIP2929 charged since address not present in access list
				uint8(evm.PUSH1), 0xe3, uint8(evm.SLOAD),
			},
			gasConsumed: uint64(46529),
		},
		{
			name: "Test 3: Check EXTCODECOPY opcode",
			code: []byte{
				// extcodecopy( 0xff,0,0,0,0)
				uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00,
				uint8(evm.PUSH1), 0xff, uint8(evm.EXTCODECOPY),
				// extcodecopy( 0xff,0,0,0,0)
				uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00,
				uint8(evm.PUSH1), 0xff, uint8(evm.EXTCODECOPY),
				// extcodecopy( this,0,0,0,0)
				uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00, uint8(evm.PUSH1), 0x00,
				uint8(evm.ADDRESS), uint8(evm.EXTCODECOPY),
				uint8(evm.STOP),
			},
			gasConsumed: uint64(2835),
		},
		{
			name: "Test 4: Check Call opcodes",
			code: []byte{
				// Precompile `0x02`
				uint8(evm.PUSH1), 0x0, uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1),
				uint8(evm.PUSH1), 0x02, uint8(evm.PUSH1), 0x0, uint8(evm.CALL), uint8(evm.POP),

				// Call `0xce`
				uint8(evm.PUSH1), 0x0, uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1),
				uint8(evm.PUSH1), 0xce, uint8(evm.PUSH1), 0x0, uint8(evm.CALL), uint8(evm.POP),

				// Delegate Call `0xce`
				uint8(evm.PUSH1), 0x0, uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1),
				uint8(evm.PUSH1), 0xce, uint8(evm.PUSH1), 0x0, uint8(evm.DELEGATECALL), uint8(evm.POP),

				// Static Call `0xbb`
				uint8(evm.PUSH1), 0x0, uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1), uint8(evm.DUP1),
				uint8(evm.PUSH1), 0xbb, uint8(evm.PUSH1), 0x0, uint8(evm.STATICCALL), uint8(evm.POP),
			},
			gasConsumed: uint64(5492),
		},
	}

	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			state := newStateWithPreState(nil)
			addr := types.BytesToAddress([]byte("contract"))
			txn := newTxn(state)
			txn.SetCode(addr, tt.code)

			enabledForks := chain.AllForksEnabled.At(0)
			transition := NewTransition(enabledForks, state, txn)

			result := transition.Call2(transition.ctx.Origin, addr, nil, big.NewInt(0), uint64(1000000))
			assert.Equal(t, tt.gasConsumed, result.GasUsed, "Gas consumption for %s is inaccurate according to EIP 2929", tt.name)
		})
	}
}
