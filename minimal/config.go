package minimal

import (
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/protocol"
)

// Config is used to parametrize the minimal client
type Config struct {
	DiscoveryBackends map[string]discovery.Factory

	ProtocolBackends map[string]protocol.Factory

	Keystore Keystore

	Consensus consensus.Consensus
}
