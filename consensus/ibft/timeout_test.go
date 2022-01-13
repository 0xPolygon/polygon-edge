package ibft

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestExponentialTimeout(t *testing.T) {
	testCases := []struct {
		description string
		exponent    uint64
		expected    time.Duration
	}{
		{"for exponent 0 returns 10s", 0, 10 * time.Second},
		{"for exponent 1 returns 12s", 1, (10 + 2) * time.Second},
		{"for exponent 2 returns 14s", 2, (10 + 4) * time.Second},
		{"for exponent 8 returns 256s", 8, (10 + 256) * time.Second},
		{"for exponent 9 returns 300s", 9, 300 * time.Second},
		{"for exponent 10 returns 300s", 10, 5 * time.Minute},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			timeout := exponentialTimeout(test.exponent)

			assert.Equal(t, test.expected, timeout)
		})
	}
}
