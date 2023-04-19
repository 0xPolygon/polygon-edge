package tracer

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// RuntimeHost is the interface defining the methods for accessing state by tracer
type RuntimeHost interface {
	// GetRefund returns refunded value
	GetRefund() uint64
	// GetStorage access the storage slot at the given address and slot hash
	GetStorage(types.Address, types.Hash) types.Hash
}

type VMState interface {
	// Halt tells VM to terminate its process
	Halt()
}

type Tracer interface {
	// Cancel tells termination of execution and tracing
	Cancel(error)
	// Clear clears the tracked data
	Clear()
	// GetResult returns a result based on tracked data
	GetResult() (interface{}, error)

	// Tx-level
	TxStart(gasLimit uint64)
	TxEnd(gasLeft uint64)

	// Call-level
	CallStart(
		depth int, // begins from 1
		from, to types.Address,
		callType int,
		gas uint64,
		value *big.Int,
		input []byte,
	)
	CallEnd(
		depth int, // begins from 1
		output []byte,
		err error,
	)

	// Op-level
	CaptureState(
		memory []byte,
		stack []*big.Int,
		opCode int,
		contractAddress types.Address,
		sp int,
		host RuntimeHost,
		state VMState,
	)
	ExecuteState(
		contractAddress types.Address,
		ip uint64,
		opcode string,
		availableGas uint64,
		cost uint64,
		lastReturnData []byte,
		depth int,
		err error,
		host RuntimeHost,
	)
}
