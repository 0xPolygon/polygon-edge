package minimal

import (
	consensusDev "github.com/0xPolygon/minimal/consensus/dev"
	consensusDummy "github.com/0xPolygon/minimal/consensus/dummy"
	consensusIBFT "github.com/0xPolygon/minimal/consensus/ibft"

	"github.com/0xPolygon/minimal/consensus"
)

var consensusBackends = map[string]consensus.Factory{
	// "ethash": consensusEthash.Factory,
	"dev":   consensusDev.Factory,
	"ibft":  consensusIBFT.Factory,
	"dummy": consensusDummy.Factory,
}
