package minimal

import (
	"net"

	"github.com/0xPolygon/minimal/chain"
)

// Config is used to parametrize the minimal client
type Config struct {
	ConsensusConfig map[string]interface{}

	Chain *chain.Chain

	JSONRPCAddr *net.TCPAddr
	GRPCAddr    *net.TCPAddr
	LibP2PAddr  *net.TCPAddr

	DataDir string
	Seal    bool
}

func DefaultConfig() *Config {
	return &Config{
		JSONRPCAddr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8545},
		GRPCAddr:        &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9632},
		LibP2PAddr:      &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1478},
		ConsensusConfig: map[string]interface{}{},
	}
}
