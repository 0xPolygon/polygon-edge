package sam

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/bridge/utils"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
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

func newTestMessage(hash types.Hash) *Message {
	return &Message{
		Hash: hash,
		Data: []byte{0x01},
	}
}

func newTestMessageSignature(hash types.Hash, address types.Address) *MessageSignature {
	return &MessageSignature{
		Hash:      hash,
		Address:   address,
		Signature: []byte{0x01},
	}
}

func newTestMessageSignaturesStore(signatures []MessageSignature) *messageSignaturesStore {
	store := newMessageSignaturesStore()

	for _, sig := range signatures {
		sig := sig
		store.Put(&sig)
	}

	return store
}

func newTestMessagePool(
	t *testing.T,
	validatorSet utils.ValidatorSet,
	knownMessages []Message,
	consumedHashes []types.Hash,
	signatures []MessageSignature,
) *pool {
	t.Helper()

	testPool := NewPool(validatorSet)

	for _, msg := range knownMessages {
		msg := msg
		testPool.AddMessage(&msg)
	}

	for _, consumed := range consumedHashes {
		testPool.ConsumeMessage(consumed)
	}

	for _, signature := range signatures {
		signature := signature
		testPool.AddSignature(&signature)
	}

	//nolint:forcetypeassert
	return testPool.(*pool)
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
		knownMessages  []Message
		consumedHashes []types.Hash
		// inputs
		signatures []MessageSignature
		// outputs
		numReadyMessages int
	}{
		{
			name: "shouldn't promote message if the message doesn't have enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 2,
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
				*newTestMessage(mockHashes[1]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
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
			knownMessages:  nil,
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
			},
			consumedHashes: []types.Hash{
				mockHashes[0],
			},
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
				*newTestMessage(mockHashes[1]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[2]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
				*newTestMessage(mockHashes[1]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
			},
			numReadyMessages: 0,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			pool := newTestMessagePool(
				t,
				utils.NewValidatorSet(testCase.validators, testCase.threshold),
				testCase.knownMessages,
				testCase.consumedHashes,
				testCase.signatures,
			)

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
		signatures []MessageSignature
		// inputs
		newKnownMessage []Message
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
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
			},
			newKnownMessage:  nil,
			numReadyMessages: 0,
		},
		{
			name: "should promote message if the pool has known after putting enough signatures",
			validators: []types.Address{
				mockAddresses[0],
				mockAddresses[1],
			},
			threshold: 1,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[1], mockAddresses[1]),
			},
			newKnownMessage: []Message{
				*newTestMessage(mockHashes[0]),
			},
			numReadyMessages: 1,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			pool := newTestMessagePool(
				t,
				utils.NewValidatorSet(testCase.validators, testCase.threshold),
				nil,
				nil,
				testCase.signatures,
			)

			for _, message := range testCase.newKnownMessage {
				message := message
				pool.AddMessage(&message)
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
		knownMessages       []Message
		consumedHashes      []types.Hash
		signatures          []MessageSignature
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
				*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
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
			knownMessages: []Message{
				*newTestMessage(mockHashes[0]),
			},
			consumedHashes: nil,
			signatures: []MessageSignature{
				*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
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
			valSet := utils.NewValidatorSet(testCase.validators, testCase.threshold)
			pool := newTestMessagePool(
				t,
				valSet,
				testCase.knownMessages,
				testCase.consumedHashes,
				testCase.signatures,
			)

			// check current ready messages
			assert.Len(t, pool.GetReadyMessages(), testCase.oldNumReadyMessages)

			// try to change validators without subscription
			valSet.SetValidators(testCase.newValidators, testCase.newThreshold)
			pool.updateValidatorSet(
				common.DiffAddresses(testCase.validators, testCase.newValidators),
				testCase.threshold,
				testCase.newThreshold,
			)

			// get result
			assert.Len(t, pool.GetReadyMessages(), testCase.newNumReadyMessages)
		})
	}
}

// Tests for messageSignaturesStore
func Test_messageSignaturesStore_HasMessage(t *testing.T) {
	store := newTestMessageSignaturesStore([]MessageSignature{
		*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
	})

	assert.True(t, store.HasMessage(mockHashes[0]), "should return true for the existing message, but got false")
	assert.False(t, store.HasMessage(mockHashes[1]), "should return false for non-existing message, but got true")
}

func Test_messageSignaturesStore_GetSignatureCount(t *testing.T) {
	store := newTestMessageSignaturesStore([]MessageSignature{
		*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
		*newTestMessageSignature(mockHashes[0], mockAddresses[0]), // signature by same address
		*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
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
	store := newTestMessageSignaturesStore([]MessageSignature{
		*newTestMessageSignature(mockHashes[0], mockAddresses[0]),
		*newTestMessageSignature(mockHashes[0], mockAddresses[1]),
		*newTestMessageSignature(mockHashes[1], mockAddresses[2]),
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
		store.GetSignatures(hash),
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

	store.Put(newTestMessageSignature(mockHashes[0], mockAddresses[0]))

	// after putting
	assert.NotNil(t,
		store.GetSignatures(hash),
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
	store := newTestMessageSignaturesStore([]MessageSignature{
		*newTestMessageSignature(hash, mockAddresses[0]),
	})

	// before removing
	assert.NotNil(t,
		store.GetSignatures(hash),
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
		store.GetSignatures(hash),
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
	store := newTestMessageSignaturesStore([]MessageSignature{
		*newTestMessageSignature(hash, mockAddresses[0]),
		*newTestMessageSignature(hash, mockAddresses[1]),
		*newTestMessageSignature(hash, mockAddresses[2]),
	})

	// before removing signatures
	assert.NotNil(t,
		store.GetSignatures(hash),
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
		store.GetSignatures(hash),
		"should return message",
	)
	assert.Equal(
		t,
		uint64(1),
		store.GetSignatureCount(hash),
		"should return only 1 signatures after removing signatures",
	)
}
