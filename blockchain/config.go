package blockchain

import "github.com/0xPolygon/polygon-edge/chain"

// Config is the configuration for the blockchain
type Config struct {
	Chain   *chain.Chain // the reference to the chain configuration
	DataDir string       // the base data directory for the client
}
