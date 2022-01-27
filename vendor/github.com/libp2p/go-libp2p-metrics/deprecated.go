// Deprecated: use github.com/libp2p/go-libp2p-core/metrics instead.
package metrics

import core "github.com/libp2p/go-libp2p-core/metrics"

// Deprecated: use github.com/libp2p/go-libp2p-core/metrics.Reporter instead.
type Reporter = core.Reporter

// Deprecated: use github.com/libp2p/go-libp2p-core/metrics.Stats instead.
type Stats = core.Stats

// Deprecated: use github.com/libp2p/go-libp2p-core/metrics.BandwidthCounter instead.
type BandwidthCounter = core.BandwidthCounter

// Deprecated: use github.com/libp2p/go-libp2p-core/metrics.NewBandwidthCounter instead.
func NewBandwidthCounter() *core.BandwidthCounter {
	return core.NewBandwidthCounter()
}
