package minimal

import (
	"crypto/ecdsa"

	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/protocol"
)

// Config is used to parametrize the minimal client
type Config struct {
	DiscoveryBackends map[string]discovery.Factory

	ProtocolBackends map[string]protocol.Factory

	Key   *ecdsa.PrivateKey
	Chain *chain.Chain

	BindAddr string
	BindPort int

	DataDir string

	Consensus consensus.Consensus

	ServiceName string
	Seal        bool
}
