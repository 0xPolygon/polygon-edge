package runtime

import (
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/types"
)

// EVMLogger is used to collect execution traces from an EVM transaction
// execution. CaptureState is called for each step of the VM with the
// current VM state.
// Note that reference types are actual VM data structures; make copies
// if you need to retain them beyond the current call.
type EVMLogger interface {
	// Transaction level
	CaptureTxStart(gasLimit uint64)
	CaptureTxEnd(restGas uint64)
	// Top call frame
	// capturestart 需要传入一个接口代表任何类型！需要有一个txn
	CaptureStart(txn interface{}, from types.Address, to types.Address, create bool, input []byte, gas uint64, value *big.Int)
	CaptureEnd(output []byte, gasUsed uint64, t time.Duration, err error)
	// Rest of call frames
	CaptureEnter(typ int, from types.Address, to types.Address, input []byte, gas uint64, value *big.Int)
	CaptureExit(output []byte, gasUsed uint64, err error)
	// Opcode level
	CaptureState(pc uint64, op int, gas, cost uint64, scope *ScopeContext, rData []byte, depth int, err error)
	CaptureFault(pc uint64, op int, gas, cost uint64, scope *ScopeContext, depth int, err error)
}
