package evm

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func newMockContract(value *big.Int, gas uint64, code []byte, to types.Address) *runtime.Contract {
	return runtime.NewContract(
		1,
		types.ZeroAddress,
		types.ZeroAddress,
		to,
		value,
		gas,
		code,
	)
}

// mockHost is a struct which meets the requirements of runtime.Host interface but throws panic in each methods
// we don't test all opcodes in this test
type mockHost struct {
	tracer runtime.VMTracer
}

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

func (m *mockHost) Transfer(from types.Address, to types.Address, amount *big.Int) error {
	panic("Not implemented in tests")
}

func (m *mockHost) GetTracer() runtime.VMTracer {
	return m.tracer
}

func (m *mockHost) GetRefund() uint64 {
	panic("Not implemented in tests")
}

func TestRun(t *testing.T) {
	t.Parallel()

	// for AUTH test ...
	// r: 0x8b1b4920a872ff8fdd66a35ef370a9d9113af4234b91ab0087c51e8362287073
	// s: 0x68da649ea8d0444671dc2e3ed9b40ab9b0eee08d745a5527bdb92f0dff746d9c
	// v: false
	// commit: 0x6ca2b29e76ec69ab132c23acdafc5650de9f0ee2aa6ada70031962b37e24b026
	// user: 0x3D09c91F44C87C30901dDB742D99f168F5AEEf01
	// invoker: 0xC66298c7a6aDE36b928d6e9598Af7804611AbDC0

	authCode := new(bytes.Buffer)
	authCode.WriteByte(PUSH32)
	authCode.Write(hex.MustDecodeHex("0x68da649ea8d0444671dc2e3ed9b40ab9b0eee08d745a5527bdb92f0dff746d9c"))
	authCode.WriteByte(PUSH32)
	authCode.Write(hex.MustDecodeHex("0x8b1b4920a872ff8fdd66a35ef370a9d9113af4234b91ab0087c51e8362287073"))
	authCode.WriteByte(PUSH1)
	authCode.WriteByte(0x00)
	authCode.WriteByte(PUSH32)
	authCode.Write(hex.MustDecodeHex("0x6ca2b29e76ec69ab132c23acdafc5650de9f0ee2aa6ada70031962b37e24b026"))
	authCode.WriteByte(AUTH)

	tests := []struct {
		name     string
		value    *big.Int
		gas      uint64
		code     []byte
		to       types.Address
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
				GasUsed:     24,
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
				GasUsed:     5000,
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
				GasUsed:     6,
				// gas consumed for 2 push1 ops
				GasLeft: 4994,
				Err:     errRevert,
			},
		},
		{
			name:  "should succeed and return value of auth signer",
			value: big.NewInt(0),
			gas:   3100 + 4*3,
			code:  authCode.Bytes(),
			to:    types.StringToAddress("0xC66298c7a6aDE36b928d6e9598Af7804611AbDC0"),
			config: &chain.ForksInTime{
				Byzantium: true,
			},
			expected: &runtime.ExecutionResult{
				ReturnValue: hex.MustDecodeHex("0x0000000000000000000000003D09c91F44C87C30901dDB742D99f168F5AEEf01"),
				GasUsed:     3100 + 4*3,
				GasLeft:     0,
				Err:         nil,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			evm := NewEVM()
			contract := newMockContract(tt.value, tt.gas, tt.code, tt.to)
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

type mockCall struct {
	name string
	args map[string]interface{}
}

type mockTracer struct {
	calls []mockCall
}

func (m *mockTracer) CaptureState(
	memory []byte,
	stack []*big.Int,
	opCode int,
	contractAddress types.Address,
	sp int,
	_host tracer.RuntimeHost,
	_state tracer.VMState,
) {
	m.calls = append(m.calls, mockCall{
		name: "CaptureState",
		args: map[string]interface{}{
			"memory":          memory,
			"stack":           stack,
			"opCode":          opCode,
			"contractAddress": contractAddress,
			"sp":              sp,
		},
	})
}

func (m *mockTracer) ExecuteState(
	contractAddress types.Address,
	ip uint64,
	opcode string,
	availableGas uint64,
	cost uint64,
	lastReturnData []byte,
	depth int,
	err error,
	_host tracer.RuntimeHost,
) {
	m.calls = append(m.calls, mockCall{
		name: "ExecuteState",
		args: map[string]interface{}{
			"contractAddress": contractAddress,
			"ip":              ip,
			"opcode":          opcode,
			"availableGas":    availableGas,
			"cost":            cost,
			"lastReturnData":  lastReturnData,
			"depth":           depth,
			"err":             err,
		},
	})
}

func TestRunWithTracer(t *testing.T) {
	t.Parallel()

	contractAddress := types.StringToAddress("1")

	tests := []struct {
		name     string
		value    *big.Int
		gas      uint64
		code     []byte
		config   *chain.ForksInTime
		expected []mockCall
	}{
		{
			name:  "should call CaptureState and ExecuteState",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				PUSH1,
				0x1,
			},
			expected: []mockCall{
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory":          []byte{},
						"stack":           []*big.Int{},
						"opCode":          int(PUSH1),
						"contractAddress": contractAddress,
						"sp":              0,
					},
				},
				{
					name: "ExecuteState",
					args: map[string]interface{}{
						"contractAddress": contractAddress,
						"ip":              uint64(0),
						"opcode":          opCodeToString[PUSH1],
						"availableGas":    uint64(5000),
						"cost":            uint64(3),
						"lastReturnData":  []byte{},
						"depth":           1,
						"err":             (error)(nil),
					},
				},
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory": []byte{},
						"stack": []*big.Int{
							big.NewInt(1),
						},
						"opCode":          int(0),
						"contractAddress": contractAddress,
						"sp":              1,
					},
				},
			},
		},
		{
			name:  "should exit with error",
			value: big.NewInt(0),
			gas:   5000,
			code: []byte{
				POP,
			},
			expected: []mockCall{
				{
					name: "CaptureState",
					args: map[string]interface{}{
						"memory":          []byte{},
						"stack":           []*big.Int{},
						"opCode":          int(POP),
						"contractAddress": contractAddress,
						"sp":              0,
					},
				},
				{
					name: "ExecuteState",
					args: map[string]interface{}{
						"contractAddress": contractAddress,
						"ip":              uint64(0),
						"opcode":          opCodeToString[POP],
						"availableGas":    uint64(5000),
						"cost":            uint64(2),
						"lastReturnData":  []byte{},
						"depth":           1,
						"err":             errStackUnderflow,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		tt := tt

		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			contract := newMockContract(tt.value, tt.gas, tt.code, types.ZeroAddress)
			contract.Address = contractAddress
			tracer := &mockTracer{}
			host := &mockHost{
				tracer: tracer,
			}
			config := tt.config
			if config == nil {
				config = &chain.ForksInTime{}
			}

			state := acquireState()
			state.resetReturnData()
			state.msg = contract
			state.code = contract.Code
			state.gas = contract.Gas
			state.host = host
			state.config = config

			// make sure stack, memory, and returnData are empty
			state.stack = make([]*big.Int, 0)
			state.memory = make([]byte, 0)
			state.returnData = make([]byte, 0)

			_, _ = state.Run()

			assert.Equal(t, tt.expected, tracer.calls)
		})
	}
}
