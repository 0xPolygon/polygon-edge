package minimal

import (
	consensusDev "github.com/0xPolygon/minimal/consensus/dev"
	consensusEthash "github.com/0xPolygon/minimal/consensus/ethash"
	consensusPOW "github.com/0xPolygon/minimal/consensus/pow"

	"github.com/0xPolygon/minimal/consensus"
)

var consensusBackends = map[string]consensus.Factory{
	"ethash": consensusEthash.Factory,
	"pow":    consensusPOW.Factory,
	"dev":    consensusDev.Factory,
	// "ibft":   consensusIBFT.Factory,
}
