package tracer

import (
	"math/big"

	"github.com/0xPolygon/polygon-edge/types"
)

// RuntimeHost is the interface defining the methods for accessing state by tracer
type RuntimeHost interface {
	// GetRefund returns refunded value
	GetRefund() uint64
	// GetStorage access the storage slot at the given address and slot ha
	GetStorage(types.Address, types.Hash) types.Hash
}

type Tracer interface {
	Clear()
	GetResult() interface{}

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
		gasUsed uint64,
		err error,
	)

	// Op-level
	CaptureState(
		// memory
		memory []byte,
		// stack
		stack []*big.Int,
		// storage
		opCode int,
		contractAddress types.Address,
		sp int,
		host RuntimeHost,
	)
	ExecuteState(
		contractAddress types.Address,
		ip int,
		opcode string,
		availableGas uint64,
		cost uint64,
		// TODO: add context
		lastReturnData []byte,
		depth int,
		err error,
		host RuntimeHost,
	)
}
