package discovery

import (
	"time"

	"github.com/libp2p/go-libp2p/core/discovery"
)

// DiscoveryOpt is a single discovery option.
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Option instead
type Option = discovery.Option

// DiscoveryOpts is a set of discovery options.
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Options instead
type Options = discovery.Options

// TTL is an option that provides a hint for the duration of an advertisement
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.TTL instead
func TTL(ttl time.Duration) Option {
	return discovery.TTL(ttl)
}

// Limit is an option that provides an upper bound on the peer count for discovery
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Limit instead
func Limit(limit int) Option {
	return discovery.Limit(limit)
}
