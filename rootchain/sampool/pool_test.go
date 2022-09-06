package sampool

import (
	"errors"
	"github.com/0xPolygon/polygon-edge/rootchain"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSAMPool_AddMessage(t *testing.T) {
	t.Parallel()

	t.Run(
		"bad hash",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash: func(msg rootchain.SAM) error {
					return errors.New("asdasd")
				},
			}

			pool := New(verifier)

			msg := rootchain.SAM{
				//Hash: [32]byte("some really bad hash"),
			}

			err := pool.AddMessage(msg)

			assert.Error(t, err)
		},
	)

	t.Run(
		"bad signature",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash: func(sam rootchain.SAM) error {
					return nil
				},

				verifySignature: func(sam rootchain.SAM) error {
					return errors.New("some really bad signature")
				},
			}

			pool := New(verifier)

			msg := rootchain.SAM{
				Signature: []byte("some really bad signature"),
			}

			err := pool.AddMessage(msg)

			assert.Error(t, err)
		},
	)

	t.Run(
		"reject stale message",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash: func(sam rootchain.SAM) error {
					return nil
				},
				verifySignature: func(sam rootchain.SAM) error {
					return nil
				},
				quorumFunc: nil,
			}

			pool := New(verifier)
			pool.lastProcessedMessage = 10

			msg := rootchain.SAM{
				Event: rootchain.Event{
					Number: 5,
				},
			}

			err := pool.AddMessage(msg)

			assert.ErrorIs(t, err, ErrStaleMessage)
		},
	)

	t.Run(
		"message accepted",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash:      func(rootchain.SAM) error { return nil },
				verifySignature: func(rootchain.SAM) error { return nil },
			}

			pool := New(verifier)

			msg := rootchain.SAM{
				Hash: types.Hash{1, 2, 3},
				Event: rootchain.Event{
					Number: 3,
				},
			}

			err := pool.AddMessage(msg)

			assert.NoError(t, err)

			bucket := pool.messagesByNumber[msg.Number]
			assert.True(t, bucket.exists(msg))
		},
	)
}

func TestSAMPool_Prune(t *testing.T) {
	t.Parallel()

	t.Run(
		"prune removes message",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash:      func(rootchain.SAM) error { return nil },
				verifySignature: func(rootchain.SAM) error { return nil },
			}

			pool := New(verifier)

			msg := rootchain.SAM{
				Hash: types.Hash{1, 2, 3},
				Event: rootchain.Event{
					Number: 3,
				},
			}

			err := pool.AddMessage(msg)
			assert.NoError(t, err)

			bucket := pool.messagesByNumber[msg.Number]
			assert.True(t, bucket.exists(msg))

			pool.Prune(5)

			bucket = pool.messagesByNumber[msg.Number]
			assert.False(t, bucket.exists(msg))
		},
	)

	t.Run(
		"prune removes no message",
		func(t *testing.T) {
			t.Parallel()

			verifier := mockVerifier{
				verifyHash:      func(rootchain.SAM) error { return nil },
				verifySignature: func(rootchain.SAM) error { return nil },
			}

			pool := New(verifier)

			msg := rootchain.SAM{
				Hash: types.Hash{1, 2, 3},
				Event: rootchain.Event{
					Number: 10,
				},
			}

			err := pool.AddMessage(msg)
			assert.NoError(t, err)

			bucket := pool.messagesByNumber[msg.Number]
			assert.True(t, bucket.exists(msg))

			pool.Prune(5)

			bucket = pool.messagesByNumber[msg.Number]
			assert.True(t, bucket.exists(msg))
		},
	)

}
