package ibft

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExponentialTimeout(t *testing.T) {
	testCases := []struct {
		description string
		exponent    uint64
		baseTimeout time.Duration
		expected    time.Duration
	}{
		{"for exponent 0 returns 10s", 0, 10 * time.Second, 10 * time.Second},
		{"for exponent 1 returns 12s", 1, 10 * time.Second, (10 + 2) * time.Second},
		{"for exponent 2 returns 14s", 2, 10 * time.Second, (10 + 4) * time.Second},
		{"for exponent 8 returns 256s", 8, 10 * time.Second, (10 + 256) * time.Second},
		{"for exponent 9 returns 300s", 9, 10 * time.Second, 300 * time.Second},
		{"for exponent 10 returns 300s", 10, 10 * time.Second, 5 * time.Minute},
		{"for block timeout 20s, exponent 0 returns 20s", 0, 20 * time.Second, 20 * time.Second},
		{"for block timeout 20s, exponent 10 returns 600s", 10, 20 * time.Second, 10 * time.Minute},
	}

	for _, test := range testCases {
		t.Run(test.description, func(t *testing.T) {
			timeout := exponentialTimeout(test.exponent, test.baseTimeout)

			assert.Equal(t, test.expected, timeout)
		})
	}
}
