package rootchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type testPayload struct {
	payloadType PayloadType
	payload     []byte
}

func (tp *testPayload) Get() (PayloadType, []byte) {
	return tp.payloadType, tp.payload
}

func TestEventToProto(t *testing.T) {
	eventPayload := []byte("payload")
	event := &Event{
		Index:       1,
		BlockNumber: 10,
		Payload: &testPayload{
			payloadType: 10,
			payload:     eventPayload,
		},
	}

	proto := event.toProto()

	assert.Equal(t, event.BlockNumber, proto.BlockNumber)
	assert.Equal(t, event.Index, proto.Index)
	assert.Equal(t, eventPayload, proto.Payload)
}

func TestEventMarshal(t *testing.T) {
	event := &Event{
		Payload: &testPayload{},
	}

	eventHash, err := event.GetHash()

	assert.NoError(t, err)
	assert.NotNil(t, eventHash)
}
