// Deprecated: use github.com/libp2p/go-libp2p-core/sec/insecure instead.
package insecure

import (
	"github.com/libp2p/go-libp2p-core/peer"
	core "github.com/libp2p/go-libp2p-core/sec/insecure"
)

// Deprecated: use github.com/libp2p/go-libp2p-core/sec/insecure.ID instead.
const ID = core.ID

// Deprecated: use github.com/libp2p/go-libp2p-core/sec/insecure.Transport instead.
type Transport = core.Transport

// Deprecated: use github.com/libp2p/go-libp2p-core/sec/insecure.New instead.
func New(id peer.ID) *core.Transport {
	return core.New(id)
}

// Deprecated: use github.com/libp2p/go-libp2p-core/sec/insecure.Conn instead.
type Conn = core.Conn
