package tracer

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/0xPolygon/polygon-edge/state/runtime"
	"github.com/0xPolygon/polygon-edge/state/runtime/evm"
	"github.com/0xPolygon/polygon-edge/types"
)

type StructLog struct {
	Pc            uint64                    `json:"pc"`
	Op            int                       `json:"op"`
	Gas           uint64                    `json:"gas"`
	GasCost       uint64                    `json:"gasCost"`
	Memory        []byte                    `json:"memory,omitempty"`
	MemorySize    int                       `json:"memSize"`
	Stack         []*big.Int                `json:"stack"`
	ReturnData    []byte                    `json:"returnData,omitempty"`
	Storage       map[types.Hash]types.Hash `json:"storage"`
	Depth         int                       `json:"depth"`
	RefundCounter uint64                    `json:"refund"`
	Err           error                     `json:"err"`
}

type StructTracer struct {
	Config Config

	logs        []StructLog
	gasLimit    uint64
	consumedGas uint64
	output      []byte
	err         error
	storage     map[types.Address]map[types.Hash]types.Hash

	currentMemory []byte
	currentStack  []*big.Int
}

func NewStructTracer() *StructTracer {
	return &StructTracer{
		Config: Config{
			EnableMemory:     true,
			DisableStack:     false,
			DisableStorage:   false,
			EnableReturnData: true,
			Limit:            0,
		},
		storage: make(map[types.Address]map[types.Hash]types.Hash),
	}
}

func (t *StructTracer) Clear() {
	t.logs = t.logs[:0]
	t.gasLimit = 0
	t.consumedGas = 0
	t.output = t.output[:0]
	t.err = nil
	t.storage = make(map[types.Address]map[types.Hash]types.Hash)
	t.currentMemory = t.currentMemory[:0]
	t.currentStack = t.currentStack[:0]
}

func (t *StructTracer) TxStart(gasLimit uint64) {
	t.gasLimit = gasLimit
}

func (t *StructTracer) TxEnd(gasLeft uint64) {
	t.consumedGas = t.gasLimit - gasLeft
}

func (t *StructTracer) CallStart(
	from, to types.Address,
	callType runtime.CallType,
	gas uint64,
	value *big.Int,
	input []byte,
) {
}

func (t *StructTracer) CallEnd(
	output []byte,
	_gasUsed uint64,
	err error,
) {
	t.output = output
	t.err = err
}

func (t *StructTracer) InnerCallStart(
	typ runtime.CallType,
	from, to types.Address,
	gas uint64,
	value *big.Int,
	input []byte,
) {
	// NOTHING TO DO
}

func (t *StructTracer) InnerCallEnd(
	output []byte,
	gasUsed uint64,
	err error,
) {
	// NOTHING TO DO
}

func (t *StructTracer) CaptureMemory(mem []byte) {
	if !t.Config.EnableMemory {
		return
	}

	// always allocate new space to get new reference
	t.currentMemory = make([]byte, len(mem))

	copy(t.currentMemory, mem)
}

func (t *StructTracer) CaptureStack(stack []*big.Int) {
	if t.Config.DisableStack {
		return
	}

	t.currentStack = make([]*big.Int, len(stack))

	for i, v := range stack {
		t.currentStack[i] = new(big.Int).Set(v)
	}
}

func (t *StructTracer) CaptureStorage(
	opcode int,
	contractAddress types.Address,
	stack []*big.Int,
	sp int,
	host runtime.Host,
) {
	if t.Config.DisableStorage || (opcode != evm.SLOAD && opcode != evm.SSTORE) {
		return
	}

	if _, ok := t.storage[contractAddress]; !ok {
		t.storage[contractAddress] = make(map[types.Hash]types.Hash)
	}

	switch opcode {
	case evm.SLOAD:
		if sp < 1 {
			break
		}

		slot := types.BytesToHash(stack[sp-1].Bytes())
		value := host.GetStorage(contractAddress, slot)

		t.storage[contractAddress][slot] = value

	case evm.SSTORE:
		if sp < 2 {
			break
		}

		slot := types.BytesToHash(stack[sp-2].Bytes())
		value := types.BytesToHash(stack[sp-1].Bytes())

		t.storage[contractAddress][slot] = value
	}
}

func (t *StructTracer) ExecuteState(
	contractAddress types.Address,
	ip int,
	opcode int,
	availableGas uint64,
	cost uint64,
	lastReturnData []byte,
	depth int,
	err error,
	host runtime.Host,
) {
	if !t.canAppendLog() {
		return
	}

	var (
		memory     []byte
		memorySize int
		stack      []*big.Int
		returnData []byte
		storage    map[types.Hash]types.Hash
	)

	if t.Config.EnableMemory {
		memory = t.currentMemory
		memorySize = len(memory)
	}

	if !t.Config.DisableStack {
		stack = t.currentStack
	}

	if t.Config.EnableReturnData {
		returnData = lastReturnData
	}

	if !t.Config.DisableStorage {
		contractStorage, ok := t.storage[contractAddress]
		if ok {
			storage = make(map[types.Hash]types.Hash, len(contractStorage))

			for k, v := range contractStorage {
				storage[k] = v
			}
		}
	}

	t.logs = append(
		t.logs,
		StructLog{
			Pc:            uint64(ip),
			Op:            opcode,
			Gas:           availableGas,
			GasCost:       cost,
			Memory:        memory,
			MemorySize:    memorySize,
			Stack:         stack,
			ReturnData:    returnData,
			Storage:       storage,
			Depth:         depth,
			RefundCounter: host.GetRefund(),
			Err:           err,
		},
	)
}

func (t *StructTracer) ExecuteFault(
	ip int,
	opcode int,
	availableGas uint64,
	cost uint64,
	depth int,
	err error,
) {
	// NOTHING TO DO
}

type StructTraceResult struct {
	Gas         uint64      `json:"gas"`
	Failed      bool        `json:"failed"`
	ReturnValue string      `json:"return_value"`
	StructLogs  []StructLog `json:"logs"`
}

func (t *StructTracer) GetResult() interface{} {
	var returnValue string

	if t.err != nil && !errors.Is(t.err, runtime.ErrExecutionReverted) {
		returnValue = ""
	} else {
		returnValue = fmt.Sprintf("%x", t.output)
	}

	return &StructTraceResult{
		Gas:         t.consumedGas,
		Failed:      t.err != nil,
		ReturnValue: returnValue,
		StructLogs:  t.logs,
	}
}

func (t *StructTracer) canAppendLog() bool {
	return t.Config.Limit == 0 || len(t.logs) < t.Config.Limit
}
