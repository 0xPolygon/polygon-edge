package txpool

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func Test_memoryTxErrorJournal_logDiscardedTx(t *testing.T) {
	t.Parallel()

	var (
		reason    = "test reason"
		strTxHash = "1"
	)

	journal, err := newMemoryTxErrorJournal()
	assert.NoError(t, err)

	journal.logDiscardedTx(types.StringToHash(strTxHash), reason)

	res, err := journal.GetReason(types.StringToHash(strTxHash))

	assert.NoError(t, err)
	assert.Equal(t, &reason, res)
}

func Test_memoryTxErrorJournalGetReason(t *testing.T) {
	t.Parallel()

	var (
		txHash1 = types.StringToHash("1")
		txHash2 = types.StringToHash("2")

		reason1 = "reason1"
		reason2 = "reason2"
	)

	tests := []struct {
		name           string
		initialReasons map[types.Hash]string
		hash           types.Hash
		expected       *string
		err            bool
	}{
		{
			name: "should return reason",
			initialReasons: map[types.Hash]string{
				txHash1: reason1,
				txHash2: reason2,
			},
			hash:     txHash1,
			expected: &reason1,
			err:      false,
		},
		{
			name: "should return nil",
			initialReasons: map[types.Hash]string{
				txHash1: reason1,
			},
			hash:     txHash2,
			expected: nil,
			err:      false,
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			journal, err := newMemoryTxErrorJournal()
			assert.NoError(t, err)

			for k, v := range test.initialReasons {
				journal.logDiscardedTx(k, v)
			}

			res, err := journal.GetReason(test.hash)

			assert.Equal(t, test.expected, res)

			if test.err {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
