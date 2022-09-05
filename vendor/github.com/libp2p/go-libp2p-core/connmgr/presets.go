package connmgr

import (
	"time"

	"github.com/libp2p/go-libp2p/core/connmgr"
)

// DecayNone applies no decay.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayNone instead
func DecayNone() DecayFn {
	return connmgr.DecayNone()
}

// DecayFixed subtracts from by the provided minuend, and deletes the tag when
// first reaching 0 or negative.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayFixed instead
func DecayFixed(minuend int) DecayFn {
	return connmgr.DecayFixed(minuend)
}

// DecayLinear applies a fractional coefficient to the value of the current tag,
// rounding down via math.Floor. It erases the tag when the result is zero.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayLinear instead
func DecayLinear(coef float64) DecayFn {
	return connmgr.DecayLinear(coef)
}

// DecayExpireWhenInactive expires a tag after a certain period of no bumps.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayExpireWhenInactive instead
func DecayExpireWhenInactive(after time.Duration) DecayFn {
	return connmgr.DecayExpireWhenInactive(after)
}

// BumpSumUnbounded adds the incoming value to the peer's score.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.BumpSumUnbounded instead
func BumpSumUnbounded() BumpFn {
	return connmgr.BumpSumUnbounded()
}

// BumpSumBounded keeps summing the incoming score, keeping it within a
// [min, max] range.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.BumpSumBounded instead
func BumpSumBounded(min, max int) BumpFn {
	return connmgr.BumpSumBounded(min, max)
}

// BumpOverwrite replaces the current value of the tag with the incoming one.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.BumpOverwrite instead
func BumpOverwrite() BumpFn {
	return connmgr.BumpOverwrite()
}
