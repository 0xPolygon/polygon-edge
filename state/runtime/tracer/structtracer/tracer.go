package structtracer

import (
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/0xPolygon/polygon-edge/helper/hex"
	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/state/runtime/tracer"
	"github.com/0xPolygon/polygon-edge/types"
)

type Config struct {
	EnableMemory     bool // enable memory capture
	EnableStack      bool // enable stack capture
	EnableStorage    bool // enable storage capture
	EnableReturnData bool // enable return data capture
	EnableStructLogs bool // enable struct logs capture
}

type StructLog struct {
	Pc            uint64            `json:"pc"`
	Op            string            `json:"op"`
	Gas           uint64            `json:"gas"`
	GasCost       uint64            `json:"gasCost"`
	Depth         int               `json:"depth"`
	Error         string            `json:"error,omitempty"`
	Stack         []string          `json:"stack,omitempty"`
	Memory        []string          `json:"memory,omitempty"`
	Storage       map[string]string `json:"storage,omitempty"`
	RefundCounter uint64            `json:"refund,omitempty"`
	ReturnData    string            `json:"returnData,omitempty"`
}

type StructTracer struct {
	Config Config

	cancelLock sync.RWMutex
	reason     error
	interrupt  bool

	logs        []StructLog
	gasLimit    uint64
	consumedGas uint64
	output      []byte
	err         error

	storage       []map[types.Address]map[types.Hash]types.Hash
	currentMemory [][]byte
	currentStack  [][]*big.Int
}

func NewStructTracer(config Config) *StructTracer {
	return &StructTracer{
		Config:     config,
		cancelLock: sync.RWMutex{},
		storage: []map[types.Address]map[types.Hash]types.Hash{
			{},
		},
		currentMemory: make([][]byte, 1),
		currentStack:  make([][]*big.Int, 1),
	}
}

func (t *StructTracer) Cancel(err error) {
	t.cancelLock.Lock()
	defer t.cancelLock.Unlock()

	t.reason = err
	t.interrupt = true
}

func (t *StructTracer) cancelled() bool {
	t.cancelLock.RLock()
	defer t.cancelLock.RUnlock()

	return t.interrupt
}

func (t *StructTracer) Clear() {
	t.cancelLock.Lock()
	defer t.cancelLock.Unlock()

	t.reason = nil
	t.interrupt = false
	t.logs = t.logs[:0]
	t.gasLimit = 0
	t.consumedGas = 0
	t.output = t.output[:0]
	t.err = nil
	t.storage = []map[types.Address]map[types.Hash]types.Hash{
		{},
	}
	t.currentMemory = make([][]byte, 1)
	t.currentStack = make([][]*big.Int, 1)
}

func (t *StructTracer) TxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

func (t *StructTracer) TxEnd(gasLeft uint64) {
	t.consumedGas = t.gasLimit - gasLeft
}

func (t *StructTracer) CallStart(
	depth int,
	from, to types.Address,
	callType int,
	gas uint64,
	value *big.Int,
	input []byte,
) {
}

func (t *StructTracer) CallEnd(
	depth int,
	output []byte,
	err error,
) {
	if depth == 1 {
		t.output = output
		t.err = err
	}
}

func (t *StructTracer) CaptureState(
	memory []byte,
	stack []*big.Int,
	opCode int,
	contractAddress types.Address,
	sp int,
	host tracer.RuntimeHost,
	state tracer.VMState,
) {
	if t.cancelled() {
		state.Halt()

		return
	}

	t.captureMemory(memory, opCode)
	t.captureStack(stack, sp, opCode)
	t.captureStorage(
		stack,
		opCode,
		contractAddress,
		sp,
		host,
	)
}

func (t *StructTracer) captureMemory(
	memory []byte,
	opCode int,
) {
	if !t.Config.EnableMemory {
		return
	}

	// always allocate new space to get new reference
	currentMemory := make([]byte, len(memory))
	copy(currentMemory, memory)

	t.currentMemory[len(t.currentMemory)-1] = currentMemory

	if opCode == evm.CALL || opCode == evm.STATICCALL {
		t.currentMemory = append(t.currentMemory, nil)
	}
}

func (t *StructTracer) captureStack(
	stack []*big.Int,
	sp int,
	opCode int,
) {
	if !t.Config.EnableStack {
		return
	}

	currentStack := make([]*big.Int, sp)

	for i, v := range stack[:sp] {
		currentStack[i] = new(big.Int).Set(v)
	}

	t.currentStack[len(t.currentStack)-1] = currentStack

	if opCode == evm.CALL || opCode == evm.STATICCALL {
		t.currentStack = append(t.currentStack, nil)
	}
}

func (t *StructTracer) captureStorage(
	stack []*big.Int,
	opCode int,
	contractAddress types.Address,
	sp int,
	host tracer.RuntimeHost,
) {
	if !t.Config.EnableStorage {
		return
	}

	addToStorage := func(key, value types.Hash) {
		if submap, initialized := t.storage[len(t.storage)-1][contractAddress]; !initialized {
			t.storage[len(t.storage)-1][contractAddress] = map[types.Hash]types.Hash{
				key: value,
			}
		} else {
			submap[key] = value
		}
	}

	switch opCode {
	case evm.SLOAD:
		if sp >= 1 {
			slot := types.BytesToHash(stack[sp-1].Bytes())
			value := host.GetStorage(contractAddress, slot)

			addToStorage(slot, value)
		}

	case evm.SSTORE:
		if sp >= 2 {
			slot := types.BytesToHash(stack[sp-1].Bytes())
			value := types.BytesToHash(stack[sp-2].Bytes())

			addToStorage(slot, value)
		}

	case evm.CALL, evm.STATICCALL:
		t.storage = append(t.storage, map[types.Address]map[types.Hash]types.Hash{})
	}
}

func (t *StructTracer) ExecuteState(
	contractAddress types.Address,
	ip uint64,
	opCode string,
	availableGas uint64,
	cost uint64,
	lastReturnData []byte,
	depth int,
	err error,
	host tracer.RuntimeHost,
) {
	var (
		errStr     string
		memory     []string
		stack      []string
		returnData string
		storage    map[string]string
		isCallOp   bool = opCode == evm.OpCode(evm.CALL).String() || opCode == evm.OpCode(evm.STATICCALL).String()
	)

	if t.Config.EnableMemory {
		if isCallOp {
			t.currentMemory = t.currentMemory[:len(t.currentMemory)-1]
		}

		size := 32
		currMemory := t.currentMemory[len(t.currentMemory)-1]
		memory = make([]string, 0, len(currMemory)/size)

		for i := 0; i+size <= len(currMemory); i += size {
			memory = append(memory, hex.EncodeToString(currMemory[i:i+size]))
		}
	}

	if t.Config.EnableStack {
		if isCallOp {
			t.currentStack = t.currentStack[:len(t.currentStack)-1]
		}

		currStack := t.currentStack[len(t.currentStack)-1]
		stack = make([]string, len(currStack))

		for i, v := range currStack {
			stack[i] = hex.EncodeBig(v)
		}
	}

	if t.Config.EnableStorage {
		if isCallOp {
			t.storage = t.storage[:len(t.storage)-1]
		}

		if contractStorage, ok := t.storage[len(t.storage)-1][contractAddress]; ok {
			storage = make(map[string]string, len(contractStorage))

			for k, v := range contractStorage {
				storage[hex.EncodeToString(k.Bytes())] = hex.EncodeToString(v.Bytes())
			}
		}
	}

	if t.Config.EnableReturnData && len(lastReturnData) > 0 {
		returnData = hex.EncodeToString(lastReturnData)
	}

	if err != nil {
		errStr = err.Error()
	}

	if t.Config.EnableStructLogs {
		t.logs = append(
			t.logs,
			StructLog{
				Pc:            ip,
				Op:            opCode,
				Gas:           availableGas,
				GasCost:       cost,
				Memory:        memory,
				Stack:         stack,
				ReturnData:    returnData,
				Storage:       storage,
				Depth:         depth,
				RefundCounter: host.GetRefund(),
				Error:         errStr,
			},
		)
	}
}

type StructTraceResult struct {
	Failed      bool        `json:"failed"`
	Gas         uint64      `json:"gas"`
	ReturnValue string      `json:"returnValue"`
	StructLogs  []StructLog `json:"structLogs"`
}

func (t *StructTracer) GetResult() (interface{}, error) {
	t.cancelLock.RLock()
	defer t.cancelLock.RUnlock()

	if t.reason != nil {
		return nil, t.reason
	}

	var returnValue string

	if t.err != nil && !errors.Is(t.err, runtime.ErrExecutionReverted) {
		returnValue = ""
	} else {
		returnValue = fmt.Sprintf("%x", t.output)
	}

	return &StructTraceResult{
		Failed:      t.err != nil,
		Gas:         t.consumedGas,
		ReturnValue: returnValue,
		StructLogs:  t.logs,
	}, nil
}
