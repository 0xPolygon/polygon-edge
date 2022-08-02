package evm

import (
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func newMockContract(value *big.Int, gas uint64, code []byte) *runtime.Contract {
	return runtime.NewContract(
		1,
		types.ZeroAddress,
		types.ZeroAddress,
		types.ZeroAddress,
		value,
		gas,
		code,
	)
}

// mockHost is a struct which meets the requirements of runtime.Host interface but throws panic in each methods
// we don't test all opcodes in this test
type mockHost struct{}

func (m *mockHost) AccountExists(addr types.Address) bool {
	panic("Not implemented in tests")
}

func (m *mockHost) GetStorage(addr types.Address, key types.Hash) types.Hash {
	panic("Not implemented in tests")
}

func (m *mockHost) SetStorage(
	addr types.Address,
	key types.Hash,
	value types.Hash,
	config *chain.ForksInTime,
) runtime.StorageStatus {
	panic("Not implemented in tests")
}

func (m *mockHost) GetBalance(addr types.Address) *big.Int {
	panic("Not implemented in tests")
}

func (m *mockHost) GetCodeSize(addr types.Address) int {
	panic("Not implemented in tests")
}

func (m *mockHost) GetCodeHash(addr types.Address) types.Hash {
	panic("Not implemented in tests")
}

func (m *mockHost) GetCode(addr types.Address) []byte {
	panic("Not implemented in tests")
}

func (m *mockHost) Selfdestruct(addr types.Address, beneficiary types.Address) {
	panic("Not implemented in tests")
}

func (m *mockHost) GetTxContext() runtime.TxContext {
	panic("Not implemented in tests")
}

func (m *mockHost) GetBlockHash(number int64) types.Hash {
	panic("Not implemented in tests")
}

func (m *mockHost) EmitLog(addr types.Address, topics []types.Hash, data []byte) {
	panic("Not implemented in tests")
}

func (m *mockHost) Callx(*runtime.Contract, runtime.Host) *runtime.ExecutionResult {
	panic("Not implemented in tests")
}

func (m *mockHost) Empty(addr types.Address) bool {
	panic("Not implemented in tests")
}

func (m *mockHost) GetNonce(addr types.Address) uint64 {
	panic("Not implemented in tests")
}

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		value    *big.Int
		gas      uint64
		code     []byte
		config   *chain.ForksInTime
		expected *runtime.ExecutionResult
	}{
		{
			name:  "should succeed because of no codes",
			value: big.NewInt(0),
			gas:   5000,
			code:  []byte{},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				GasLeft:     5000,
			},
		},
		{
			name:  "should succeed and return result",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				PUSH1, 0x01, PUSH1, 0x02, ADD,
				PUSH1, 0x00, MSTORE8,
				PUSH1, 0x01, PUSH1, 0x00, RETURN,
			},
			expected: &runtime.ExecutionResult{
				ReturnValue: []uint8{0x03},
				GasLeft:     4976,
			},
		},
		{
			name:  "should fail and consume all gas by error",
			value: big.NewInt(0),
			gas:   5000,
			// ADD will be failed by stack underflow
			code: []byte{ADD},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				GasLeft:     0,
				Err:         errStackUnderflow,
			},
		},
		{
			name:  "should fail by REVERT and return remaining gas at that time",
			value: big.NewInt(0),
			gas:   5000,
			// Stack size and offset for return value first
			code: []byte{PUSH1, 0x00, PUSH1, 0x00, REVERT},
			config: &chain.ForksInTime{
				Byzantium: true,
			},
			expected: &runtime.ExecutionResult{
				ReturnValue: nil,
				// gas consumed for 2 push1 ops
				GasLeft: 4994,
				Err:     errRevert,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evm := NewEVM()
			contract := newMockContract(tt.value, tt.gas, tt.code)
			host := &mockHost{}
			config := tt.config
			if config == nil {
				config = &chain.ForksInTime{}
			}
			res := evm.Run(contract, host, config)
			assert.Equal(t, tt.expected, res)
		})
	}
}
