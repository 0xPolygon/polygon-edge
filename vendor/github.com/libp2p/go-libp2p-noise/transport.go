// Deprecated: This package has moved into go-libp2p as a sub-package: github.com/libp2p/go-libp2p/p2p/security/noise.
package noise

import (
	"github.com/libp2p/go-libp2p/p2p/security/noise"

	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/sec"
)

// ID is the protocol ID for noise
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.ID instead.
const ID = noise.ID

var _ sec.SecureTransport = &Transport{}

// Transport implements the interface sec.SecureTransport
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.Transport instead.
type Transport = noise.Transport

// New creates a new Noise transport using the given private key as its
// libp2p identity key.
// Deprecated: use github.com/libp2p/go-libp2p/p2p/security/noise.New instead.
func New(privkey crypto.PrivKey) (*Transport, error) {
	return noise.New(privkey)
}
