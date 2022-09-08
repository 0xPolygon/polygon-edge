package rootchain

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func TestSAMToProto(t *testing.T) {
	var (
		hash      = types.BytesToHash([]byte("Unique hash"))
		signature = []byte("Unique signature")
		blockNum  = uint64(1)
		event     = Event{
			Index:       1,
			BlockNumber: 10,
			Payload: &testPayload{
				payloadType: 10,
				payload:     []byte("payload"),
			},
		}
	)

	sam := &SAM{
		Hash:          hash,
		Signature:     signature,
		ChildBlockNum: blockNum,
		Event:         event,
	}

	proto := sam.ToProto()

	assert.Equal(t, hash.Bytes(), proto.Hash)
	assert.Equal(t, signature, proto.Signature)
	assert.Equal(t, blockNum, proto.ChildchainBlockNumber)

	_, eventPayload := event.Get()
	assert.Equal(t, event.Index, proto.Event.Index)
	assert.Equal(t, event.BlockNumber, proto.Event.BlockNumber)
	assert.Equal(t, eventPayload, proto.Event.Payload)
}

func TestSAMMarshal(t *testing.T) {
	sam := &SAM{
		Hash:      types.BytesToHash([]byte("Unique hash")),
		Signature: []byte("Unique signature"),
		Event: Event{
			Payload: &testPayload{},
		},
	}

	marshalled, err := sam.Marshal()

	assert.NoError(t, err)
	assert.NotNil(t, marshalled)
}
