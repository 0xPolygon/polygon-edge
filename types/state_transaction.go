package types

import (
	"fmt"
	"math/big"

	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
)

var ExecuteStateSyncABIMethod, _ = abi.NewMethod("function execute(" +
	"bytes32[] proof, " +
	"tuple(uint256 id, address sender, address receiver, bytes data, bool skip) stateSync)")

const (
	abiMethodIDLength = 4
)

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

func (sse *StateSyncEvent) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"id":       sse.ID,
		"sender":   sse.Sender,
		"receiver": sse.Receiver,
		"data":     sse.Data,
		"skip":     sse.Skip,
	}
}

func (sse *StateSyncEvent) String() string {
	return fmt.Sprintf("Id=%d, Sender=%v, Target=%v", sse.ID, sse.Sender, sse.Receiver)
}

type StateSyncProof struct {
	Proof     []Hash
	StateSync *StateSyncEvent
}

// EncodeAbi contains logic for encoding given ABI data
func (ssp *StateSyncProof) EncodeAbi() ([]byte, error) {
	return ExecuteStateSyncABIMethod.Encode([2]interface{}{ssp.Proof, ssp.StateSync.ToMap()})
}

// DecodeAbi contains logic for decoding given ABI data
func (ssp *StateSyncProof) DecodeAbi(txData []byte) error {
	if len(txData) < abiMethodIDLength {
		return fmt.Errorf("invalid proof data, len = %d", len(txData))
	}

	rawResult, err := ExecuteStateSyncABIMethod.Inputs.Decode(txData[abiMethodIDLength:])
	if err != nil {
		return err
	}

	result, isOk := rawResult.(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid proof data")
	}

	stateSyncEventEncoded, isOk := result["stateSync"].(map[string]interface{})
	if !isOk {
		return fmt.Errorf("invalid state sync data")
	}

	proofEncoded, isOk := result["proof"].([][32]byte)
	if !isOk {
		return fmt.Errorf("invalid proof data")
	}

	id, isOk := stateSyncEventEncoded["id"].(*big.Int)
	if !isOk {
		return fmt.Errorf("invalid state sync event id")
	}

	senderEthgo, isOk := stateSyncEventEncoded["sender"].(ethgo.Address)
	if !isOk {
		return fmt.Errorf("invalid state sync sender field")
	}

	receiverEthgo, isOk := stateSyncEventEncoded["receiver"].(ethgo.Address)
	if !isOk {
		return fmt.Errorf("invalid state sync receiver field")
	}

	data, isOk := stateSyncEventEncoded["data"].([]byte)
	if !isOk {
		return fmt.Errorf("invalid state sync data field")
	}

	skip, isOk := stateSyncEventEncoded["skip"].(bool)
	if !isOk {
		return fmt.Errorf("invalid state sync skip field")
	}

	stateSync := &StateSyncEvent{
		ID:       id.Uint64(),
		Sender:   senderEthgo,
		Receiver: receiverEthgo,
		Data:     data,
		Skip:     skip,
	}

	proof := make([]Hash, len(proofEncoded))
	for i := 0; i < len(proofEncoded); i++ {
		proof[i] = Hash(proofEncoded[i])
	}

	*ssp = StateSyncProof{
		Proof:     proof,
		StateSync: stateSync,
	}

	return nil
}
