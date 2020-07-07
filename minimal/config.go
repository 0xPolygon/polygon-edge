package minimal

import (
	"github.com/0xPolygon/minimal/api"
	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/chain"
	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/minimal/keystore"
	"github.com/0xPolygon/minimal/network/discovery"
	"github.com/0xPolygon/minimal/protocol"
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

	APIBackends map[string]api.Factory
	APIEntries  map[string]*Entry

	Keystore keystore.Keystore
	Chain    *chain.Chain

	BindAddr string
	BindPort int

	RPCAddr string
	RPCPort int

	DataDir     string
	ServiceName string
	Seal        bool

	StateStorage string
}
