// Deprecated: use github.com/libp2p/go-libp2p-core/pnet instead.
package ipnet

import core "github.com/libp2p/go-libp2p-core/pnet"

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.EnvKey instead.
const EnvKey = core.EnvKey

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.ForcePrivateNetwork instead.
// Warning: it's impossible to alias a var in go. Writes to this var would have no longer
// have any effect, so it has been commented out to induce breakage for added safety.
// var ForcePrivateNetwork = core.ForcePrivateNetwork

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.ErrNotInPrivateNetwork instead.
var ErrNotInPrivateNetwork = core.ErrNotInPrivateNetwork

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.Protector instead.
type Protector = core.Protector

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.Error instead.
type PNetError = core.Error

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.NewError instead.
func NewError(err string) error {
	return core.NewError(err)
}

// Deprecated: use github.com/libp2p/go-libp2p-core/pnet.IsPNetError instead.
func IsPNetError(err error) bool {
	return core.IsPNetError(err)
}
