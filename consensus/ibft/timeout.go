package ibft

import (
	"math"
	"time"
)

const (
	maxTimeoutMultiplier = 30
)

// exponentialTimeout calculates the timeout duration in seconds as exponential function
// where maximum value returned can't exceed 30 * baseTimeout
// t = baseTimeout * maxTimeoutMultiplier where exponent > 8
// t = baseTimeout + 2^exponent	          where exponent > 0
// t = baseTimeout			  where exponent = 0
func exponentialTimeout(exponent uint64, baseTimeout time.Duration) time.Duration {
	maxTimeout := baseTimeout * maxTimeoutMultiplier
	if exponent > 8 {
		return maxTimeout
	}

	timeout := baseTimeout
	if exponent > 0 {
		timeout += time.Duration(math.Pow(2, float64(exponent))) * time.Second
	}

	return timeout
}
