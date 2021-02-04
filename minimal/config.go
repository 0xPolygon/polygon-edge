package minimal

import (
	"net"

	"github.com/0xPolygon/minimal/chain"
)

// Config is used to parametrize the minimal client
type Config struct {
	//DiscoveryBackends map[string]discovery.Factory
	//DiscoveryEntries  map[string]*Entry

	//ProtocolBackends map[string]protocol.Factory
	//ProtocolEntries  map[string]*Entry

	//BlockchainBackends map[string]storage.Factory
	//BlockchainEntries  map[string]*Entry

	// ConsensusBackends map[string]consensus.Factory
	ConsensusConfig map[string]interface{}

	//APIBackends map[string]api.Factory
	//APIEntries  map[string]*Entry

	// Keystore keystore.Keystore
	Chain *chain.Chain

	GRPCAddr   *net.TCPAddr
	LibP2PAddr *net.TCPAddr

	DataDir string
	Seal    bool
}

func DefaultConfig() *Config {
	return &Config{
		GRPCAddr:        &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9632},
		LibP2PAddr:      &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1478},
		ConsensusConfig: map[string]interface{}{},
	}
}
