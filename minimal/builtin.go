package minimal

import (
	consensusDev "github.com/0xPolygon/minimal/consensus/dev"
	consensusIBFT "github.com/0xPolygon/minimal/consensus/ibft"

	"github.com/0xPolygon/minimal/consensus"
)

// consensusBackends ties in the consensus Factory methods
var consensusBackends = map[string]consensus.Factory{
	// "ethash": consensusEthash.Factory,
	"dev":  consensusDev.Factory,
	"ibft": consensusIBFT.Factory,
}
