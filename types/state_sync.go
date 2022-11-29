package types

import (
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var ExecuteBundleABIMethod, _ = abi.NewMethod("function execute(" +
	"bytes32[] proof, " +
	"tuple(uint256 id, address sender, address receiver, bytes data, bool skip)[] objs)")

// StateSyncEvent is a bridge event from the rootchain
type StateSyncEvent struct {
	// ID is the decoded 'index' field from the event
	ID uint64
	// Sender is the decoded 'sender' field from the event
	Sender ethgo.Address
	// Receiver is the decoded 'receiver' field from the event
	Receiver ethgo.Address
	// Data is the decoded 'data' field from the event
	Data []byte
	// Skip is the decoded 'skip' field from the event
	Skip bool
}

type StateSyncProof struct {
	Proof     []Hash
	StateSync StateSyncEvent
}
