package ibft

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIBFTBackend_CalculateHeaderTimestamp verifies that the header timestamp
// is successfully calculated
func TestIBFTBackend_CalculateHeaderTimestamp(t *testing.T) {
	t.Parallel()

	// Reference time
	now := time.Unix(time.Now().UTC().Unix(), 0) // Round down

	testTable := []struct {
		name            string
		parentTimestamp int64
		currentTime     time.Time
		blockTime       uint64

		expectedTimestamp time.Time
	}{
		{
			"Valid clock block timestamp",
			now.Add(time.Duration(-1) * time.Second).Unix(), // 1s before
			now,
			1,
			now, // 1s after
		},
		{
			"Next multiple block clock",
			now.Add(time.Duration(-4) * time.Second).Unix(), // 4s before
			now,
			3,
			roundUpTime(now, 3*time.Second),
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			i := &backendIBFT{
				blockTime: time.Duration(testCase.blockTime) * time.Second,
			}

			assert.Equal(
				t,
				testCase.expectedTimestamp.Unix(),
				i.calcHeaderTimestamp(
					uint64(testCase.parentTimestamp),
					testCase.currentTime,
				).Unix(),
			)
		})
	}
}

// TestIBFTBackend_RoundUpTime verifies time is rounded up correctly
func TestIBFTBackend_RoundUpTime(t *testing.T) {
	t.Parallel()

	// Reference time
	now := time.Now().UTC()

	calcExpected := func(time int64, multiple int64) int64 {
		if time%multiple == 0 {
			return time + multiple
		}

		return ((time + multiple/2) / multiple) * multiple
	}

	testTable := []struct {
		name     string
		time     time.Time
		multiple time.Duration

		expectedTime int64
	}{
		{
			"No rounding needed",
			now,
			0 * time.Second,
			now.Unix(),
		},
		{
			"Rounded up time even",
			now,
			2 * time.Second,
			calcExpected(now.Unix(), 2),
		},
	}

	for _, testCase := range testTable {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(
				t,
				testCase.expectedTime,
				roundUpTime(testCase.time, testCase.multiple).Unix(),
			)
		})
	}
}
