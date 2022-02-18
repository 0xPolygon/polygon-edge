package sam

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

var (
	mockAddresses = []types.Address{
		types.StringToAddress("1"),
		types.StringToAddress("2"),
		types.StringToAddress("3"),
	}

	mockHashes = []types.Hash{
		types.BytesToHash(crypto.Keccak256([]byte{0x01})),
		types.BytesToHash(crypto.Keccak256([]byte{0x02})),
		types.BytesToHash(crypto.Keccak256([]byte{0x03})),
	}
)

func newTestSignedMessage(hash types.Hash, address types.Address) *SignedMessage {
	return &SignedMessage{
		Message:   []byte{},
		Hash:      hash,
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

func getMessageHashes(store *messageSignaturesStore) []types.Hash {
	hashes := make([]types.Hash, 0)

	store.RangeMessages(func(entry *signedMessageEntry) bool {
		hashes = append(hashes, entry.Hash)

		return true
	})

	return hashes
}

// Tests for Pool
func Test_Pool_Add(t *testing.T) {
	tests := []struct {
		name string
		// initial state
		validators     []types.Address
		threshold      uint64
		knownHashes    []types.Hash
		consumedHashes []types.Hash
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
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
			},
			numReadyMessages: 0,
		},
		{
			name: "should promote message if the message has enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
				mockHashes[1],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			numReadyMessages: 1,
		},
		{
			name: "shouldn't promote message if the message is not known",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold:      2,
			knownHashes:    nil,
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			numReadyMessages: 0,
		},
		{
			name: "shouldn't promote message if the message has been consumed",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
			},
			consumedHashes: []types.Hash{
				mockHashes[0],
			},
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			numReadyMessages: 0,
		},
		{
			name: "should promote multiple messages",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
				mockHashes[1],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[2]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			numReadyMessages: 2,
		},
		{
			name: "shouldn't promote for multi signing",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
				mockHashes[1],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
			},
			numReadyMessages: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// initialize
			pool := NewPool(testCase.validators, testCase.threshold)
			for _, hash := range testCase.knownHashes {
				pool.MarkAsKnown(hash)
			}

			for _, hash := range testCase.consumedHashes {
				pool.Consume(hash)
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
		validators []types.Address
		threshold  uint64
		messages   []SignedMessage
		// inputs
		newKnownHashes []types.Hash
		// outputs
		numReadyMessages int
	}{
		{
			name: "shouldn't promote message if the messages are not known",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 1,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			newKnownHashes:   nil,
			numReadyMessages: 0,
		},
		{
			name: "should promote message if the pool has known after putting enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 1,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[1], mockAddresses[1]),
			},
			newKnownHashes: []types.Hash{
				mockHashes[0],
			},
			numReadyMessages: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// initialize
			pool := NewPool(testCase.validators, testCase.threshold)

			for _, msg := range testCase.messages {
				msg := msg
				pool.Add(&msg)
			}

			for _, hash := range testCase.newKnownHashes {
				pool.MarkAsKnown(hash)
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
		knownHashes         []types.Hash
		consumedHashes      []types.Hash
		messages            []SignedMessage
		oldNumReadyMessages int
		// inputs
		newValidators []types.Address
		newThreshold  uint64
		// outputs
		newNumReadyMessages int
	}{
		{
			name: "should demote if the validator that has signed message left from validator set",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
				*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
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
			threshold: 1,
			knownHashes: []types.Hash{
				mockHashes[0],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
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
			threshold: 2,
			knownHashes: []types.Hash{
				mockHashes[0],
			},
			consumedHashes: nil,
			messages: []SignedMessage{
				*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
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
			for _, id := range testCase.knownHashes {
				pool.MarkAsKnown(id)
			}

			for _, id := range testCase.consumedHashes {
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
		*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
	})

	assert.True(t, store.HasMessage(mockHashes[0]), "should return true for the existing message, but got false")
	assert.False(t, store.HasMessage(mockHashes[1]), "should return false for non-existing message, but got true")
}

func Test_messageSignaturesStore_GetSignatureCount(t *testing.T) {
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
		*newTestSignedMessage(mockHashes[0], mockAddresses[0]), // signature by same address
		*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
	})

	assert.Equal(
		t,
		uint64(2),
		store.GetSignatureCount(mockHashes[0]),
		"should return number of signatures in store for the existing message",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(mockHashes[1]),
		"should return zero for non-existing message",
	)
}

func Test_messageSignaturesStore_RangeMessage(t *testing.T) {
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(mockHashes[0], mockAddresses[0]),
		*newTestSignedMessage(mockHashes[0], mockAddresses[1]),
		*newTestSignedMessage(mockHashes[1], mockAddresses[2]),
	})
	expected := map[types.Hash][]types.Address{
		mockHashes[0]: {
			mockAddresses[0],
			mockAddresses[1],
		},
		mockHashes[1]: {
			mockAddresses[2],
		},
	}
	actual := make(map[types.Hash][]types.Address)

	store.RangeMessages(func(entry *signedMessageEntry) bool {
		addresses := make([]types.Address, 0, entry.NumSignatures())

		entry.Signatures.Range(func(key, value interface{}) bool {
			address, ok := key.(types.Address)
			assert.True(t, ok)

			addresses = append(addresses, address)

			return true
		})

		actual[entry.Hash] = addresses

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

	for hash, expectedAddrs := range expected {
		assert.ElementsMatch(
			t,
			expectedAddrs,
			actual[hash],
			"Store should have addresses who has signed messages, but missing some addresses for %s",
			hash.String(),
		)
	}
}

func Test_messageSignaturesStore_PutMessage(t *testing.T) {
	store := newTestMessageSignaturesStore(nil)
	hash := mockHashes[0]

	// before putting
	assert.Nil(t,
		store.GetMessage(hash),
		"should return nil as default",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(hash),
		"should return zero message as default",
	)
	assert.Len(
		t,
		getMessageHashes(store),
		0,
		"should not have any keys as default",
	)

	store.PutMessage(newTestSignedMessage(mockHashes[0], mockAddresses[0]))

	// after putting
	assert.NotNil(t,
		store.GetMessage(hash),
		"should return entry after putting message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(hash),
		"should return number of signatures after putting message",
	)
	assert.Len(
		t,
		getMessageHashes(store),
		1,
		"should have only one key after putting one message",
	)
}

func Test_messageSignaturesStore_RemoveMessage(t *testing.T) {
	hash := mockHashes[0]
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(hash, mockAddresses[0]),
	})

	// before removing
	assert.NotNil(t,
		store.GetMessage(hash),
		"should return the entry before removing message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(hash),
		"should return number of signatures in store before removing message",
	)
	assert.Len(
		t,
		getMessageHashes(store),
		1,
		"should have only one key in store after before removing message",
	)

	assert.Truef(
		t,
		store.RemoveMessage(hash),
		"RemoveMessage should return true for existing message, but got false",
	)

	// after removing
	assert.Nil(
		t,
		store.GetMessage(hash),
		"should return nil after removing",
	)
	assert.Equal(
		t,
		uint64(0),
		store.GetSignatureCount(hash),
		"should return zero message after removing",
	)
	assert.Len(
		t,
		getMessageHashes(store),
		0,
		"should not have any keys after removing",
	)
}

func Test_messageSignaturesStore_RemoveSignatures(t *testing.T) {
	hash := mockHashes[0]
	store := newTestMessageSignaturesStore([]SignedMessage{
		*newTestSignedMessage(hash, mockAddresses[0]),
		*newTestSignedMessage(hash, mockAddresses[1]),
		*newTestSignedMessage(hash, mockAddresses[2]),
	})

	// before removing signatures
	assert.NotNil(t,
		store.GetMessage(hash),
		"should return message as default",
	)
	assert.Equal(
		t,
		uint64(3),
		store.GetSignatureCount(hash),
		"should return 3 signatures as default",
	)

	store.RemoveSignatures([]types.Address{
		mockAddresses[0],
		mockAddresses[1],
	})

	// after removing signatures
	assert.NotNil(
		t,
		store.GetMessage(hash),
		"should return message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(hash),
		"should return only 1 signatures after removing signatures",
	)
}
