package rootchain

import "github.com/0xPolygon/polygon-edge/types"

//	TODO: define iota types
type PayloadType uint8

type Payload interface {
	Get() (PayloadType, []byte)
}

type Event struct {
	Number      uint64 // index of the event emitted from the rootchain contract
	BlockNumber uint64 // number of the block the event was contained in

	Payload // event specific data
}

type SAM struct {
	Hash      types.Hash // unique hash of the event
	Signature []byte     // validator signature

	Event
}

type VerifiedSAM []SAM

func (v VerifiedSAM) Signatures() (signatures [][]byte) {
	for _, m := range v {
		signatures = append(signatures, m.Signature)
	}

	return
}
