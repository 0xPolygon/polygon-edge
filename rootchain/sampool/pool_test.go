package sampool

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSAMPool_AddMessage(t *testing.T) {
	t.Parallel()

	t.Run(
		"ErrStaleMessage",
		func(t *testing.T) {
			t.Parallel()

			pool := New()
			pool.lastProcessedIndex = 10

			assert.ErrorIs(t,
				pool.AddMessage(
					rootchain.SAM{
						Event: rootchain.Event{Index: 3},
					},
				),
				ErrStaleMessage,
			)
		},
	)

	t.Run(
		"message added",
		func(t *testing.T) {
			t.Parallel()

			pool := New()

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 3,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))

			bucket, ok := pool.messages[msg.Index]
			assert.True(t, ok)
			assert.NotNil(t, bucket)

			set, ok := bucket[msg.Hash]
			assert.True(t, ok)
			assert.NotNil(t, set)

			messages := set.getMessages()
			assert.NotNil(t, messages)
			assert.Len(t, messages, 1)
		},
	)

	t.Run(
		"no duplicate message added",
		func(t *testing.T) {
			t.Parallel()

			pool := New()

			msg := rootchain.SAM{
				Hash:      types.Hash{111},
				Signature: []byte("signature"),
				Event: rootchain.Event{
					Index: 3,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))

			bucket, ok := pool.messages[msg.Index]
			assert.True(t, ok)
			assert.NotNil(t, bucket)

			set, ok := bucket[msg.Hash]
			assert.True(t, ok)
			assert.NotNil(t, set)

			messages := set.getMessages()
			assert.NotNil(t, messages)
			assert.Len(t, messages, 1)

			//	add the message again
			assert.NoError(t, pool.AddMessage(msg))

			//	num of messages is still 1
			set = pool.messages[msg.Index][msg.Hash]
			assert.Len(t, set.getMessages(), 1)
		},
	)
}

func TestSAMPool_Prune(t *testing.T) {
	t.Parallel()

	t.Run(
		"Prune removes message",
		func(t *testing.T) {
			t.Parallel()

			pool := New()

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 3,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))

			_, ok := pool.messages[msg.Index]
			assert.True(t, ok)

			pool.Prune(5)

			_, ok = pool.messages[msg.Index]
			assert.False(t, ok)
		},
	)

	t.Run(
		"Prune removes no message",
		func(t *testing.T) {
			t.Parallel()

			pool := New()

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 10,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))

			_, ok := pool.messages[msg.Index]
			assert.True(t, ok)

			pool.Prune(5)

			_, ok = pool.messages[msg.Index]
			assert.True(t, ok)
		},
	)
}

func TestSAMPool_Peek(t *testing.T) {
	t.Parallel()

	t.Run(
		"Peek returns nil (no message)",
		func(t *testing.T) {
			t.Parallel()

			pool := New()
			pool.lastProcessedIndex = 3

			assert.Nil(t, pool.Peek())
		},
	)

	t.Run(
		"Peek returns verified SAM",
		func(t *testing.T) {
			t.Parallel()

			pool := New()
			pool.lastProcessedIndex = 9

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 10,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))
			assert.NotNil(t, pool.Peek())
		},
	)
}

func TestSAMPool_Pop(t *testing.T) {
	t.Parallel()

	t.Run(
		"Pop returns nil (no message)",
		func(t *testing.T) {
			t.Parallel()

			pool := New()

			assert.Nil(t, pool.Pop())
		},
	)

	t.Run(
		"Pop returns VerifiedSAM",
		func(t *testing.T) {
			t.Parallel()

			pool := New()
			pool.lastProcessedIndex = 4

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 5,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))
			assert.NotNil(t, pool.Pop())

			_, ok := pool.messages[msg.Index]
			assert.False(t, ok)
			assert.Equal(t, uint64(5), pool.lastProcessedIndex)
		},
	)
}
