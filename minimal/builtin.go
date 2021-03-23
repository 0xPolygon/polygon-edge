package minimal

import (
	consensusEthash "github.com/0xPolygon/minimal/consensus/ethash"
	consensusIBFT "github.com/0xPolygon/minimal/consensus/ibft"
	consensusPOW "github.com/0xPolygon/minimal/consensus/pow"

	"github.com/0xPolygon/minimal/consensus"
)

var consensusBackends = map[string]consensus.Factory{
	"ethash": consensusEthash.Factory,
	"pow":    consensusPOW.Factory,
	"ibft":   consensusIBFT.Factory,
}
