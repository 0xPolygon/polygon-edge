package txpool

import (
	"testing"

	"github.com/0xPolygon/polygon-edge/types"
	"github.com/stretchr/testify/assert"
)

func toTxStatusPtr(status TxStatus) *TxStatus {
	return &status
}

func toBoolPtr(f bool) *bool {
	return &f
}

func TestTxStatusString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   *TxStatus
		expected string
	}{
		{
			name:     "TxSuccessful",
			status:   toTxStatusPtr(TxSuccessful),
			expected: "success",
		},
		{
			name:     "TxDropped",
			status:   toTxStatusPtr(TxDropped),
			expected: "dropped",
		},
		{
			name:     "nil",
			status:   nil,
			expected: "",
		},
		{
			name:     "otherwise",
			status:   toTxStatusPtr(TxStatus(2)),
			expected: "",
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				test.expected,
				test.status.String(),
			)
		})
	}
}

var (
	testTxHash = types.StringToHash("test")
)

func assertMemoryJournalTxStatus(t *testing.T, journal *memoryJournal, txHash types.Hash, expected *bool) {
	t.Helper()

	res, ok := journal.txHashMap.Load(txHash)

	if expected != nil {
		assert.True(t, ok)
		assert.Equal(t, *expected, res.(bool)) //nolint:forcetypeassert
	} else {
		assert.False(t, ok)
		assert.Nil(t, res)
	}
}

func Test_memoruJournal_logSuccessfulTx(t *testing.T) {
	t.Parallel()

	t.Run("add new status", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()
		journal.logSuccessfulTx(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(true))
	})

	t.Run("update status", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()
		journal.txHashMap.Store(testTxHash, false)
		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(false))

		journal.logSuccessfulTx(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(true))
	})
}

func Test_memoruJournal_logDroppedTx(t *testing.T) {
	t.Parallel()

	t.Run("add new status", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()
		journal.logDroppedTx(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(false))
	})

	t.Run("update status", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()
		journal.txHashMap.Store(testTxHash, true)
		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(true))

		journal.logDroppedTx(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, toBoolPtr(false))
	})
}

func Test_memoruJournal_resetTxStatus(t *testing.T) {
	t.Parallel()

	t.Run("clear the set status", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()
		journal.logSuccessfulTx(testTxHash) // set before reset

		journal.resetTxStatus(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, nil)
	})

	t.Run("nothing happens for unexisting data", func(t *testing.T) {
		t.Parallel()

		journal := newMemoryJournal()

		journal.resetTxStatus(testTxHash)

		assertMemoryJournalTxStatus(t, journal, testTxHash, nil)
	})
}

func Test_memoruJournal_txStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		status   *bool
		expected *TxStatus
	}{
		{
			name:     "should return TxSuccessful if tx is set to true",
			status:   toBoolPtr(true),
			expected: toTxStatusPtr(TxSuccessful),
		},
		{
			name:     "should return TxDropped if tx is set to false",
			status:   toBoolPtr(false),
			expected: toTxStatusPtr(TxDropped),
		},
		{
			name:     "should return nil for not marked tx",
			status:   nil,
			expected: (*TxStatus)(nil),
		},
	}

	for _, test := range tests {
		test := test

		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			journal := newMemoryJournal()

			if test.status != nil {
				journal.txHashMap.Store(testTxHash, *test.status)
			}

			status := journal.txStatus(testTxHash)
			assert.Equal(
				t,
				test.expected,
				status,
			)
		})
	}
}
