package connmgr

import (
	"github.com/libp2p/go-libp2p/core/connmgr"
)

// Decayer is implemented by connection managers supporting decaying tags. A
// decaying tag is one whose value automatically decays over time.
//
// The actual application of the decay behaviour is encapsulated in a
// user-provided decaying function (DecayFn). The function is called on every
// tick (determined by the interval parameter), and returns either the new value
// of the tag, or whether it should be erased altogether.
//
// We do not set values on a decaying tag. Rather, we "bump" decaying tags by a
// delta. This calls the BumpFn with the old value and the delta, to determine
// the new value.
//
// Such a pluggable design affords a great deal of flexibility and versatility.
// Behaviours that are straightforward to implement include:
//
//   - Decay a tag by -1, or by half its current value, on every tick.
//   - Every time a value is bumped, sum it to its current value.
//   - Exponentially boost a score with every bump.
//   - Sum the incoming score, but keep it within min, max bounds.
//
// Commonly used DecayFns and BumpFns are provided in this package.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.Decayer instead
type Decayer = connmgr.Decayer

// DecayFn applies a decay to the peer's score. The implementation must call
// DecayFn at the interval supplied when registering the tag.
//
// It receives a copy of the decaying value, and returns the score after
// applying the decay, as well as a flag to signal if the tag should be erased.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayFn instead
type DecayFn = connmgr.DecayFn

// BumpFn applies a delta onto an existing score, and returns the new score.
//
// Non-trivial bump functions include exponential boosting, moving averages,
// ceilings, etc.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.BumpFn instead
type BumpFn = connmgr.BumpFn

// DecayingTag represents a decaying tag. The tag is a long-lived general
// object, used to operate on tag values for peers.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayingTag instead
type DecayingTag = connmgr.DecayingTag

// DecayingValue represents a value for a decaying tag.
// Deprecated: use github.com/libp2p/go-libp2p/core/connmgr.DecayingValue instead
type DecayingValue = connmgr.DecayingValue
