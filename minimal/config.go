package minimal

import (
	"github.com/umbracle/minimal/blockchain/storage"
	"github.com/umbracle/minimal/chain"
	"github.com/umbracle/minimal/consensus"
	"github.com/umbracle/minimal/minimal/keystore"
	"github.com/umbracle/minimal/network/discovery"
	"github.com/umbracle/minimal/protocol"
)

// Config is used to parametrize the minimal client
type Config struct {
	DiscoveryBackends map[string]discovery.Factory
	DiscoveryEntries  map[string]*Entry

	ProtocolBackends map[string]protocol.Factory
	ProtocolEntries  map[string]*Entry

	BlockchainBackends map[string]storage.Factory
	BlockchainEntries  map[string]*Entry

	ConsensusBackends map[string]consensus.Factory
	ConsensusEntry    *Entry

	Keystore keystore.Keystore
	Chain    *chain.Chain

	BindAddr string
	BindPort int

	DataDir     string
	ServiceName string
	Seal        bool
}
