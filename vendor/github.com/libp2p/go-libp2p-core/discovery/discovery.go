// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/core/discovery.
//
// Package discovery provides service advertisement and peer discovery interfaces for libp2p.
package discovery

import (
	"github.com/libp2p/go-libp2p/core/discovery"
)

// Advertiser is an interface for advertising services
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Advertiser instead
type Advertiser = discovery.Advertiser

// Discoverer is an interface for peer discovery
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Discoverer instead
type Discoverer = discovery.Discoverer

// Discovery is an interface that combines service advertisement and peer discovery
// Deprecated: use github.com/libp2p/go-libp2p/core/discovery.Discovery instead
type Discovery = discovery.Discovery
