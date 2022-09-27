package rootchain

import (
	rootProto "github.com/0xPolygon/polygon-edge/rootchain/proto"
	"github.com/0xPolygon/polygon-edge/types"
	"google.golang.org/protobuf/proto"
)

type SAM struct {
	Hash          types.Hash // unique hash of the event (keccak 256)
	Signature     []byte     // validator signature
	ChildBlockNum uint64     // the childchain block number when the SAM was generated

	Event
}

// ToProto converts the local data struct to a proto spec
func (s *SAM) ToProto() *rootProto.SAM {
	// Fetch the payload
	return &rootProto.SAM{
		Hash:                  s.Hash.Bytes(),
		Signature:             s.Signature,
		ChildchainBlockNumber: s.ChildBlockNum,
		Event:                 s.Event.toProto(),
	}
}

// Marshal marshals the SAM into bytes
func (s *SAM) Marshal() ([]byte, error) {
	return proto.Marshal(s.ToProto())
}
