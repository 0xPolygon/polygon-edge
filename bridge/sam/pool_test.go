package sam

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	mockAddresses = []types.Address{
		types.StringToAddress("1"),
		types.StringToAddress("2"),
		types.StringToAddress("3"),
	}
)

func newTestSignedMessage(id uint64, address types.Address) *SignedMessage {
	return &SignedMessage{
		Message: Message{
			ID: id,
		},
		Address:   address,
		Signature: nil,
	}
}

func newTestMessageSignaturesStore(msgs []SignedMessage) *messageSignaturesStore {
	store := newMessageSignaturesStore()

	for _, msg := range msgs {
		msg := msg
		store.PutMessage(&msg)
	}

	return store
}

func getMessageIDKeys(store *messageSignaturesStore) []uint64 {
	ids := make([]uint64, 0)

	store.RangeMessages(func(entry *signedMessageEntry) bool {
		ids = append(ids, entry.Message.ID)

		return true
	})

	return ids
}

// Tests for Pool
func Test_Pool_Add(t *testing.T) {
	tests := []struct {
		name string
		// initial state
		validators  []types.Address
		threshold   uint64
		knownIDs    []uint64
		consumedIDS []uint64
		// inputs
		messages []SignedMessage
		// outputs
		numReadyMessages int
	}{
		{
			name: "should promote message if the message doesn't have enough message",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
			},
			numReadyMessages: 0,
		},
		{
			name: "should promote message if the message has enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1, 2},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[1]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			numReadyMessages: 1,
		},
		{
			name: "shouldn't promote message if the message is not known",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    nil,
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[1]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			numReadyMessages: 0,
		},
		{
			name: "shouldn't promote message if the message has been consumed",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1},
			consumedIDS: []uint64{1},
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[1]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			numReadyMessages: 0,
		},
		{
			name: "should promote multiple messages",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1, 2},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[1]),
				*newTestSignedMessage(1, mockAddresses[2]),
				*newTestSignedMessage(2, mockAddresses[0]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			numReadyMessages: 2,
		},
		{
			name: "shouldn't promote for multi signing",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1, 2},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[0]),
			},
			numReadyMessages: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// initialize
			pool := NewPool(testCase.validators, testCase.threshold)
			for _, id := range testCase.knownIDs {
				pool.MarkAsKnown(id)
			}

			for _, id := range testCase.consumedIDS {
				pool.Consume(id)
			}

			// put messages
			for _, msg := range testCase.messages {
				msg := msg
				pool.Add(&msg)
			}

			// get result
			assert.Len(t, pool.GetReadyMessages(), testCase.numReadyMessages)
		})
	}
}

func Test_Pool_MarkAsKnown(t *testing.T) {
	tests := []struct {
		name string
		// initial state
		validators  []types.Address
		threshold   uint64
		knownIDs    []uint64
		consumedIDS []uint64
		messages    []SignedMessage
		// inputs
		newKnownIDs []uint64
		// outputs
		numReadyMessages int
	}{
		{
			name: "shouldn't promote message if the messages are not known",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   1,
			knownIDs:    nil,
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			newKnownIDs:      nil,
			numReadyMessages: 0,
		},
		{
			name: "should promote message if the pool has known after putting enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   1,
			knownIDs:    nil,
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(2, mockAddresses[1]),
			},
			newKnownIDs:      []uint64{1},
			numReadyMessages: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// initialize
			pool := NewPool(testCase.validators, testCase.threshold)
			for _, id := range testCase.knownIDs {
				pool.MarkAsKnown(id)
			}

			for _, id := range testCase.consumedIDS {
				pool.Consume(id)
			}

			for _, msg := range testCase.messages {
				msg := msg
				pool.Add(&msg)
			}

			for _, id := range testCase.newKnownIDs {
				pool.MarkAsKnown(id)
			}

			// get result
			assert.Len(t, pool.GetReadyMessages(), testCase.numReadyMessages)
		})
	}
}

func Test_Pool_UpdateValidatorSet(t *testing.T) {
	tests := []struct {
		name string
		// initial state
		validators          []types.Address
		threshold           uint64
		knownIDs            []uint64
		consumedIDS         []uint64
		messages            []SignedMessage
		oldNumReadyMessages int
		// inputs
		newValidators []types.Address
		newThreshold  uint64
		// outputs
		newNumReadyMessages int
	}{
		{
			name: "should demote if the validator has signed message left from set",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
				*newTestSignedMessage(1, mockAddresses[1]),
			},
			oldNumReadyMessages: 1,
			newValidators: []types.Address{
				mockAddresses[1],
				mockAddresses[2],
			},
			newThreshold:        2,
			newNumReadyMessages: 0,
		},
		{
			name: "should demote if validator threshold increases and the message doesn't have enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   1,
			knownIDs:    []uint64{1},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
			},
			oldNumReadyMessages: 1,
			newValidators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			newThreshold:        2,
			newNumReadyMessages: 0,
		},
		{
			name: "should promote if validator threshold decreases and the message has enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:   2,
			knownIDs:    []uint64{1},
			consumedIDS: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(1, mockAddresses[0]),
			},
			oldNumReadyMessages: 0,
			newValidators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			newThreshold:        1,
			newNumReadyMessages: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// initialize
			pool := NewPool(testCase.validators, testCase.threshold)
			for _, id := range testCase.knownIDs {
				pool.MarkAsKnown(id)
			}

			for _, id := range testCase.consumedIDS {
				pool.Consume(id)
			}

			for _, msg := range testCase.messages {
				msg := msg
				pool.Add(&msg)
			}

			// check current ready messages
			assert.Len(t, pool.GetReadyMessages(), testCase.oldNumReadyMessages)

			pool.UpdateValidatorSet(testCase.newValidators, testCase.newThreshold)

			// get result
			assert.Len(t, pool.GetReadyMessages(), testCase.newNumReadyMessages)
		})
	}
}

// Tests for messageSignaturesStore
func Test_messageSignaturesStore_HasMessage(t *testing.T) {
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(1, mockAddresses[0]),
	})

	assert.True(t, store.HasMessage(1), "should return true for the existing message, but got false")
	assert.False(t, store.HasMessage(2), "should return false for non-existing message, but got true")
}

func Test_messageSignaturesStore_GetSignatureCount(t *testing.T) {
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(1, mockAddresses[0]),
		*newTestSignedMessage(1, mockAddresses[0]), // signature by same address
		*newTestSignedMessage(1, mockAddresses[1]),
	})

	assert.Equal(
		t,
		uint64(2),
		store.GetSignatureCount(1),
		"should return number of signatures in store for the existing message",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(2),
		"should return zero for non-existing message",
	)
}

func Test_messageSignaturesStore_RangeMessage(t *testing.T) {
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(1, mockAddresses[0]),
		*newTestSignedMessage(1, mockAddresses[1]),
		*newTestSignedMessage(2, mockAddresses[2]),
	})
	expected := map[uint64][]types.Address{
		1: {
			mockAddresses[0],
			mockAddresses[1],
		},
		2: {
			mockAddresses[2],
		},
	}
	actual := make(map[uint64][]types.Address)

	store.RangeMessages(func(entry *signedMessageEntry) bool {
		addresses := make([]types.Address, 0, entry.NumSignatures())

		entry.Signatures.Range(func(key, value interface{}) bool {
			address, ok := key.(types.Address)
			assert.True(t, ok)

			addresses = append(addresses, address)

			return true
		})

		actual[entry.Message.ID] = addresses

		return true
	})

	assert.Equalf(
		t,
		len(expected),
		len(actual),
		"Store should have %d messages, but got %d",
		len(expected),
		len(actual),
	)

	for id, expectedAddrs := range expected {
		assert.ElementsMatch(
			t,
			expectedAddrs,
			actual[id],
			"Store should have addresses who has signed messages, but missing some addresses for ID %d",
			id,
		)
	}
}

func Test_messageSignaturesStore_PutMessage(t *testing.T) {
	store := newTestMessageSignaturesStore(nil)

	messageID := uint64(1)

	// before putting
	assert.Nil(t,
		store.GetMessage(messageID),
		"should return nil as default",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(messageID),
		"should return zero message as default",
	)
	assert.Len(
		t,
		getMessageIDKeys(store),
		0,
		"should not have any keys as default",
	)

	store.PutMessage(newTestSignedMessage(1, mockAddresses[0]))

	// after putting
	assert.NotNil(t,
		store.GetMessage(messageID),
		"should return entry after putting message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(messageID),
		"should return number of signatures after putting message",
	)
	assert.Len(
		t,
		getMessageIDKeys(store),
		1,
		"should have only one key after putting one message",
	)
}

func Test_messageSignaturesStore_RemoveMessage(t *testing.T) {
	messageID := uint64(1)
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(messageID, mockAddresses[0]),
	})

	// before removing
	assert.NotNil(t,
		store.GetMessage(messageID),
		"should return the entry before removing message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(messageID),
		"should return number of signatures in store before removing message",
	)
	assert.Len(
		t,
		getMessageIDKeys(store),
		1,
		"should have only one key in store after before removing message",
	)

	assert.Truef(
		t,
		store.RemoveMessage(messageID),
		"RemoveMessage should return true for existing message, but got false",
	)

	// after removing
	assert.Nil(
		t,
		store.GetMessage(messageID),
		"should return nil after removing",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(messageID),
		"should return zero message after removing",
	)
	assert.Len(
		t,
		getMessageIDKeys(store),
		0,
		"should not have any keys after removing",
	)
}

func Test_messageSignaturesStore_RemoveSignatures(t *testing.T) {
	messageID := uint64(1)
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(messageID, mockAddresses[0]),
		*newTestSignedMessage(messageID, mockAddresses[1]),
		*newTestSignedMessage(messageID, mockAddresses[2]),
	})

	// before removing signatures
	assert.NotNil(t,
		store.GetMessage(messageID),
		"should return message as default",
	)
	assert.Equal(
		t,
		uint64(3),
		store.GetSignatureCount(messageID),
		"should return 3 signatures as default",
	)

	store.RemoveSignatures([]types.Address{
		mockAddresses[0],
		mockAddresses[1],
	})

	// after removing signatures
	assert.NotNil(
		t,
		store.GetMessage(messageID),
		"should return message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(messageID),
		"should return only 1 signatures after removing signatures",
	)
}
