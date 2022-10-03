package sampool

import (
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSAMPool_AddMessage(t *testing.T) {
	t.Parallel()

	t.Run(
		"ErrStaleMessage",
		func(t *testing.T) {
			t.Parallel()

			pool := New(hclog.NewNullLogger())
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

			pool := New(hclog.NewNullLogger())

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

			pool := New(hclog.NewNullLogger())

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

			pool := New(hclog.NewNullLogger())

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 3,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))
			assert.Contains(t, pool.messages, msg.Index)

			pool.Prune(5)

			assert.NotContains(t, pool.messages, msg.Index)
		},
	)

	t.Run(
		"Prune removes no message",
		func(t *testing.T) {
			t.Parallel()

			pool := New(hclog.NewNullLogger())

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 10,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))
			assert.Contains(t, pool.messages, msg.Index)

			pool.Prune(5)

			assert.Contains(t, pool.messages, msg.Index)
		},
	)
}

func TestSAMPool_Peek(t *testing.T) {
	t.Parallel()

	t.Run(
		"Peek returns nil (no message)",
		func(t *testing.T) {
			t.Parallel()

			pool := New(hclog.NewNullLogger())
			pool.lastProcessedIndex = 3

			assert.Nil(t, pool.Peek())
		},
	)

	t.Run(
		"Peek returns verified SAM",
		func(t *testing.T) {
			t.Parallel()

			pool := New(hclog.NewNullLogger())
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

			pool := New(hclog.NewNullLogger())

			assert.Nil(t, pool.Pop())
		},
	)

	t.Run(
		"Pop returns VerifiedSAM",
		func(t *testing.T) {
			t.Parallel()

			pool := New(hclog.NewNullLogger())
			pool.lastProcessedIndex = 4

			msg := rootchain.SAM{
				Hash: types.Hash{111},
				Event: rootchain.Event{
					Index: 5,
				},
			}

			assert.NoError(t, pool.AddMessage(msg))
			assert.NotNil(t, pool.Pop())

			assert.NotContains(t, pool.messages, msg.Index)
			assert.Equal(t, uint64(5), pool.lastProcessedIndex)
		},
	)
}
