package rootchain

import (
	"github.com/0xPolygon/polygon-edge/crypto"
	"google.golang.org/protobuf/proto"

	rootProto "github.com/0xPolygon/polygon-edge/rootchain/proto"
)

// The Event is a generic ordered rootchain event
type Event struct {
	Index       uint64 // index of the event emitted from the rootchain contract
	BlockNumber uint64 // number of the block the event was contained in

	Payload // event specific data
}

// toProto converts the local data struct to a proto spec
func (e *Event) toProto() *rootProto.Event {
	// Fetch the payload
	_, payload := e.Get()

	return &rootProto.Event{
		Index:       e.Index,
		BlockNumber: e.BlockNumber,
		Payload:     payload,
	}
}

// marshal encodes the Event into bytes
func (e *Event) marshal() ([]byte, error) {
	return proto.Marshal(e.toProto())
}

// GetHash returns the Keccak256 hash of the event
func (e *Event) GetHash() ([]byte, error) {
	evData, err := e.marshal()
	if err != nil {
		return nil, err
	}

	return crypto.Keccak256(evData), nil
}
