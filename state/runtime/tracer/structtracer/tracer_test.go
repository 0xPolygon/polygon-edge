package structtracer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
)

var (
	testFrom = types.StringToAddress("1")
	testTo   = types.StringToAddress("2")

	testEmptyConfig = Config{}
)

type mockState struct {
	halted bool
}

func (m *mockState) Halt() {
	m.halted = true
}

type mockHost struct {
	getRefundFn    func() uint64
	getStorageFunc func(types.Address, types.Hash) types.Hash
}

func (m *mockHost) GetRefund() uint64 {
	return m.getRefundFn()
}

func (m *mockHost) GetStorage(a types.Address, h types.Hash) types.Hash {
	return m.getStorageFunc(a, h)
}

func TestStructLogErrorString(t *testing.T) {
	t.Parallel()

	errMsg := "error message"

	tests := []struct {
		name     string
		log      StructLog
		expected string
	}{
		{
			name: "should return error message",
			log: StructLog{
				Error: errors.New(errMsg).Error(),
			},
			expected: errMsg,
		},
		{
			name: "should return empty string",
			log: StructLog{
				Error: "",
			},
			expected: "",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, test.log.Error)
		})
	}
}

func TestStructTracerCancel(t *testing.T) {
	t.Parallel()

	err := errors.New("timeout")

	tracer := NewStructTracer(testEmptyConfig)

	assert.Nil(t, tracer.reason)
	assert.False(t, tracer.interrupt)

	tracer.Cancel(err)

	assert.Equal(t, err, tracer.reason)
	assert.True(t, tracer.interrupt)
}

func TestStructTracer_canceled(t *testing.T) {
	t.Parallel()

	err := errors.New("timeout")

	tracer := NewStructTracer(testEmptyConfig)

	assert.False(t, tracer.cancelled())

	tracer.Cancel(err)

	assert.True(t, tracer.cancelled())
}

func TestStructTracerClear(t *testing.T) {
	t.Parallel()

	tracer := StructTracer{
		Config: Config{
			EnableMemory:     true,
			EnableStack:      true,
			EnableStorage:    true,
			EnableReturnData: true,
			EnableStructLogs: true,
		},
		reason:    errors.New("timeout"),
		interrupt: true,
		logs: []StructLog{
			{
				Pc: 1,
			},
		},
		gasLimit:    1024,
		consumedGas: 512,
		output:      []byte("output example"),
		err:         runtime.ErrInsufficientBalance,
		storage: []map[types.Address]map[types.Hash]types.Hash{
			{
				types.StringToAddress("1"): {
					types.StringToHash("2"): types.StringToHash("3"),
				},
			},
		},
		currentMemory: [][]byte{[]byte("memory example")},
		currentStack: []([]*big.Int){[]*big.Int{
			new(big.Int).SetUint64(1),
			new(big.Int).SetUint64(2),
		}},
	}

	tracer.Clear()

	assert.Equal(
		t,
		StructTracer{
			Config: Config{
				EnableMemory:     true,
				EnableStack:      true,
				EnableStorage:    true,
				EnableReturnData: true,
				EnableStructLogs: true,
			},
			reason:      nil,
			interrupt:   false,
			logs:        []StructLog{},
			gasLimit:    0,
			consumedGas: 0,
			output:      []byte{},
			err:         nil,
			storage: []map[types.Address]map[types.Hash]types.Hash{
				make(map[types.Address]map[types.Hash]types.Hash),
			},
			currentMemory: make([]([]byte), 1),
			currentStack:  make([]([]*big.Int), 1),
		},
		tracer,
	)
}

func TestStructTracerTxStart(t *testing.T) {
	t.Parallel()

	var (
		gasLimit uint64 = 1024
	)

	tracer := NewStructTracer(testEmptyConfig)

	tracer.TxStart(gasLimit)

	assert.Equal(
		t,
		&StructTracer{
			Config: testEmptyConfig,
			storage: []map[types.Address]map[types.Hash]types.Hash{
				make(map[types.Address]map[types.Hash]types.Hash),
			},
			gasLimit:      gasLimit,
			currentMemory: make([]([]byte), 1),
			currentStack:  make([]([]*big.Int), 1),
		},
		tracer,
	)
}

func TestStructTracerTxEnd(t *testing.T) {
	t.Parallel()

	var (
		gasLimit uint64 = 1024
		gasLeft  uint64 = 256
	)

	tracer := NewStructTracer(testEmptyConfig)

	tracer.TxStart(gasLimit)
	tracer.TxEnd(gasLeft)

	assert.Equal(
		t,
		&StructTracer{
			Config: testEmptyConfig,
			storage: []map[types.Address]map[types.Hash]types.Hash{
				make(map[types.Address]map[types.Hash]types.Hash),
			},
			gasLimit:      gasLimit,
			consumedGas:   gasLimit - gasLeft,
			currentMemory: make([]([]byte), 1),
			currentStack:  make([]([]*big.Int), 1),
		},
		tracer,
	)
}

func TestStructTracerCallStart(t *testing.T) {
	t.Parallel()

	tracer := NewStructTracer(testEmptyConfig)

	tracer.CallStart(
		1,
		testFrom,
		testTo,
		2,
		1024,
		new(big.Int).SetUint64(10000),
		[]byte("input"),
	)

	// make sure the method updates nothing
	assert.Equal(
		t,
		NewStructTracer(testEmptyConfig),
		tracer,
	)
}

func TestStructTracerCallEnd(t *testing.T) {
	t.Parallel()

	var (
		output = []byte("output")
		err    = errors.New("call err")
	)

	tests := []struct {
		name     string
		depth    int
		output   []byte
		err      error
		expected *StructTracer
	}{
		{
			name:   "should set output and error if depth is 1",
			depth:  1,
			output: output,
			err:    err,
			expected: &StructTracer{
				Config: testEmptyConfig,
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
				output:        output,
				err:           err,
				currentMemory: make([]([]byte), 1),
				currentStack:  make([]([]*big.Int), 1),
			},
		},
		{
			name:   "should update nothing if depth exceeds 1",
			depth:  2,
			output: output,
			err:    err,
			expected: &StructTracer{
				Config: testEmptyConfig,
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
				currentMemory: make([]([]byte), 1),
				currentStack:  make([]([]*big.Int), 1),
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tracer := NewStructTracer(testEmptyConfig)

			tracer.CallEnd(test.depth, test.output, test.err)

			assert.Equal(
				t,
				test.expected,
				tracer,
			)
		})
	}
}

func TestStructTracerCaptureState(t *testing.T) {
	t.Parallel()

	var (
		memory = [][]byte{[]byte("memory")}
		stack  = []([]*big.Int){[]*big.Int{
			big.NewInt(1), /* value */
			big.NewInt(2), /* key */
		}}
		contractAddress = types.StringToAddress("3")
		storageValue    = types.StringToHash("4")
	)

	tests := []struct {
		name string

		// initial state
		tracer *StructTracer

		// input
		memory          [][]byte
		stack           [][]*big.Int
		opCode          int
		contractAddress types.Address
		sp              int
		host            tracer.RuntimeHost
		vmState         tracer.VMState

		// expected state
		expectedTracer  *StructTracer
		expectedVMState tracer.VMState
	}{
		{
			name: "should capture memory",
			tracer: &StructTracer{
				Config: Config{
					EnableMemory:     true,
					EnableStructLogs: true,
				},
				currentMemory: make([]([]byte), 1),
			},
			memory:          memory,
			stack:           stack,
			opCode:          1,
			contractAddress: contractAddress,
			sp:              2,
			host:            nil,
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableMemory:     true,
					EnableStructLogs: true,
				},
				currentMemory: memory,
			},
			expectedVMState: &mockState{},
		},
		{
			name: "should capture stack",
			tracer: &StructTracer{
				Config: Config{
					EnableStack:      true,
					EnableStructLogs: true,
				},
				currentStack: make([]([]*big.Int), 1),
			},
			memory:          memory,
			stack:           stack,
			opCode:          1,
			contractAddress: contractAddress,
			sp:              2,
			host:            nil,
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableStack:      true,
					EnableStructLogs: true,
				},
				currentStack: stack,
			},
			expectedVMState: &mockState{},
		},
		{
			name: "should capture storage by SLOAD",
			tracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
			},
			memory:          memory,
			stack:           stack,
			opCode:          evm.SLOAD,
			contractAddress: contractAddress,
			sp:              2,
			host: &mockHost{
				getStorageFunc: func(a types.Address, h types.Hash) types.Hash {
					assert.Equal(t, contractAddress, a)
					assert.Equal(t, types.BytesToHash(big.NewInt(2).Bytes()), h)

					return storageValue
				},
			},
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: [](map[types.Address]map[types.Hash]types.Hash){
					map[types.Address]map[types.Hash]types.Hash{
						contractAddress: {
							types.BytesToHash(big.NewInt(2).Bytes()): storageValue,
						},
					},
				},
			},
			expectedVMState: &mockState{},
		},
		{
			name: "should capture storage by SSTORE",
			tracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					{
						contractAddress: {
							types.StringToHash("100"): types.StringToHash("200"),
						},
					},
				},
			},
			memory:          memory,
			stack:           stack,
			opCode:          evm.SSTORE,
			contractAddress: contractAddress,
			sp:              2,
			host:            nil,
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					{
						contractAddress: {
							types.StringToHash("100"):                types.StringToHash("200"),
							types.BytesToHash(big.NewInt(2).Bytes()): types.BytesToHash(big.NewInt(1).Bytes()),
						},
					},
				}},
			expectedVMState: &mockState{},
		},
		{
			name: "should call Halt() if it's been canceled",
			tracer: &StructTracer{
				Config:    testEmptyConfig,
				interrupt: true,
			},
			memory:          memory,
			stack:           stack,
			opCode:          1,
			contractAddress: contractAddress,
			sp:              2,
			host:            nil,
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config:    testEmptyConfig,
				interrupt: true,
			},
			expectedVMState: &mockState{
				halted: true,
			},
		},
		{
			name: "should not capture if sp is less than 1 in case op SLOAD",
			tracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
			},
			memory:          memory,
			stack:           stack,
			opCode:          evm.SLOAD,
			contractAddress: contractAddress,
			sp:              0,
			host:            &mockHost{},
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
			},
			expectedVMState: &mockState{},
		},
		{
			name: "should not capture if sp is less than 2 in case op SSTORE",
			tracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
			},
			memory:          memory,
			stack:           stack,
			opCode:          evm.SSTORE,
			contractAddress: contractAddress,
			sp:              1,
			host:            nil,
			vmState: &mockState{
				halted: false,
			},
			expectedTracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: []map[types.Address]map[types.Hash]types.Hash{
					make(map[types.Address]map[types.Hash]types.Hash),
				},
			},
			expectedVMState: &mockState{},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.tracer.CaptureState(
				test.memory[0],
				test.stack[0],
				test.opCode,
				test.contractAddress,
				test.sp,
				test.host,
				test.vmState,
			)

			assert.Equal(
				t,
				test.expectedTracer,
				test.tracer,
			)

			assert.Equal(
				t,
				test.expectedVMState,
				test.vmState,
			)
		})
	}
}

func TestStructTracerExecuteState(t *testing.T) {
	t.Parallel()

	var (
		contractAddress = types.StringToAddress("1")
		ip              = uint64(2)
		opCode          = "ADD"
		availableGas    = uint64(1000)
		cost            = uint64(100)
		lastReturnData  = []byte("return data")
		depth           = 1
		err             = errors.New("err")
		refund          = uint64(10000)

		memory = [][]byte{
			getMemoryString("memory sample"),
		}
		storage = []map[types.Address]map[types.Hash]types.Hash{{
			contractAddress: {
				types.StringToHash("1"): types.StringToHash("2"),
				types.StringToHash("3"): types.StringToHash("4"),
			},
			types.StringToAddress("x"): {
				types.StringToHash("5"): types.StringToHash("6"),
				types.StringToHash("7"): types.StringToHash("8"),
			},
		}}
	)

	tests := []struct {
		name string

		// initial state
		tracer *StructTracer

		// input
		contractAddress types.Address
		ip              uint64
		opCode          string
		availableGas    uint64
		cost            uint64
		lastReturnData  []byte
		depth           int
		err             error
		host            tracer.RuntimeHost

		// expected result
		expected []StructLog
	}{
		{
			name: "should create no log",
			tracer: &StructTracer{
				Config: testEmptyConfig,
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: nil,
		},
		{
			name: "should create minimal log",
			tracer: &StructTracer{
				Config: Config{
					EnableStructLogs: true,
				},
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: []StructLog{
				{
					Pc:            ip,
					Op:            opCode,
					Gas:           availableGas,
					GasCost:       cost,
					Memory:        nil,
					Stack:         nil,
					ReturnData:    "",
					Storage:       nil,
					Depth:         depth,
					RefundCounter: refund,
					Error:         err.Error(),
				},
			},
		},
		{
			name: "should save memory",
			tracer: &StructTracer{
				Config: Config{
					EnableMemory:     true,
					EnableStructLogs: true,
				},
				currentMemory: memory,
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: []StructLog{
				{
					Pc:            ip,
					Op:            opCode,
					Gas:           availableGas,
					GasCost:       cost,
					Memory:        []string{hex.EncodeToString(memory[0])},
					Stack:         nil,
					ReturnData:    "",
					Storage:       nil,
					Depth:         depth,
					RefundCounter: refund,
					Error:         err.Error(),
				},
			},
		},
		{
			name: "should save stack",
			tracer: &StructTracer{
				Config: Config{
					EnableStack:      true,
					EnableStructLogs: true,
				},
				currentStack: [][]*big.Int{{
					big.NewInt(1),
					big.NewInt(2),
				}},
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: []StructLog{
				{
					Pc:      ip,
					Op:      opCode,
					Gas:     availableGas,
					GasCost: cost,
					Memory:  nil,
					Stack: []string{
						hex.EncodeBig(big.NewInt(1)),
						hex.EncodeBig(big.NewInt(2)),
					},
					ReturnData:    "",
					Storage:       nil,
					Depth:         depth,
					RefundCounter: refund,
					Error:         err.Error(),
				},
			},
		},
		{
			name: "should save return data",
			tracer: &StructTracer{
				Config: Config{
					EnableReturnData: true,
				},
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: nil,
		},
		{
			name: "should save storage",
			tracer: &StructTracer{
				Config: Config{
					EnableStorage:    true,
					EnableStructLogs: true,
				},
				storage: storage,
			},
			contractAddress: contractAddress,
			ip:              ip,
			opCode:          opCode,
			availableGas:    availableGas,
			cost:            cost,
			lastReturnData:  lastReturnData,
			depth:           depth,
			err:             err,
			host: &mockHost{
				getRefundFn: func() uint64 {
					return refund
				},
			},
			expected: []StructLog{
				{
					Pc:         ip,
					Op:         opCode,
					Gas:        availableGas,
					GasCost:    cost,
					Memory:     nil,
					Stack:      nil,
					ReturnData: "",
					Storage: map[string]string{
						hex.EncodeToString(types.StringToHash("1").Bytes()): hex.EncodeToString(types.StringToHash("2").Bytes()),
						hex.EncodeToString(types.StringToHash("3").Bytes()): hex.EncodeToString(types.StringToHash("4").Bytes()),
					},
					Depth:         depth,
					RefundCounter: refund,
					Error:         err.Error(),
				},
			},
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.tracer.ExecuteState(
				test.contractAddress,
				test.ip,
				test.opCode,
				test.availableGas,
				test.cost,
				test.lastReturnData,
				test.depth,
				test.err,
				test.host,
			)

			assert.Equal(
				t,
				test.expected,
				test.tracer.logs,
			)
		})
	}
}

func TestStructTracerGetResult(t *testing.T) {
	t.Parallel()

	t.Run("reason is not nil", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("timeout")
		res, err := (&StructTracer{reason: expectedErr}).GetResult()

		assert.ErrorIs(t, err, expectedErr)
		assert.Nil(t, res)
	})

	t.Run("return value for ErrExecutionReverted error", func(t *testing.T) {
		t.Parallel()

		res, err := (&StructTracer{err: runtime.ErrExecutionReverted, output: []byte{2}}).GetResult()

		assert.Nil(t, err)
		require.NotNil(t, res)

		stresult, ok := res.(*StructTraceResult)

		require.True(t, ok)
		require.True(t, stresult.Failed)
		require.Equal(t, "02", stresult.ReturnValue)
	})

	t.Run("return value for non ErrExecutionReverted error", func(t *testing.T) {
		t.Parallel()

		res, err := (&StructTracer{err: errors.New("op"), output: []byte{2}}).GetResult()

		assert.Nil(t, err)
		require.NotNil(t, res)

		stresult, ok := res.(*StructTraceResult)

		require.True(t, ok)
		require.True(t, stresult.Failed)
		require.Empty(t, stresult.ReturnValue)
	})

	t.Run("return value for ErrExecutionReverted error", func(t *testing.T) {
		t.Parallel()

		logs := []StructLog{
			{
				Pc:            100,
				Op:            "sload",
				Gas:           100,
				GasCost:       50,
				Memory:        []string{"hello"},
				Stack:         []string{"1", "2"},
				ReturnData:    "1",
				Storage:       map[string]string{"1": "2"},
				Depth:         1,
				RefundCounter: 100,
				Error:         "",
			},
		}
		consumedGas := uint64(1230)
		res, err := (&StructTracer{
			consumedGas: consumedGas,
			output:      []byte{2},
			logs:        logs,
		}).GetResult()

		assert.Nil(t, err)
		require.NotNil(t, res)

		stresult, ok := res.(*StructTraceResult)

		require.True(t, ok)
		require.False(t, stresult.Failed)
		require.Equal(t, "02", stresult.ReturnValue)
		require.Equal(t, logs, stresult.StructLogs)
		require.Equal(t, consumedGas, stresult.Gas)
	})
}

func getMemoryString(s string) []byte {
	if len(s) == 0 {
		return make([]byte, 32)
	} else if len(s)%32 == 0 {
		return []byte(s)
	}

	return append([]byte(s), make([]byte, 32-(len(s)%32))...)
}
