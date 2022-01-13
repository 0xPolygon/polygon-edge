package ibft

import (
	"math"
	"time"
)

const (
	baseTimeout = 10 * time.Second
	maxTimeout  = 300 * time.Second
)

// exponentialTimeout calculates the timeout duration in seconds as exponential function
// where maximum value returned can't exceed 300 seconds
// t = 10 + 2^exponent	where exponent > 0
// t = 10				where exponent = 0
func exponentialTimeout(exponent uint64) time.Duration {
	if exponent > 8 {
		return maxTimeout
	}

	timeout := baseTimeout
	if exponent > 0 {
		timeout += time.Duration(math.Pow(2, float64(exponent))) * time.Second
	}

	return timeout
}
