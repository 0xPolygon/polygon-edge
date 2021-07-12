package minimal

import (
	"github.com/0xPolygon/minimal/consensus"
	consensusDev "github.com/0xPolygon/minimal/consensus/dev"
	consensusDummy "github.com/0xPolygon/minimal/consensus/dummy"
	consensusIBFT "github.com/0xPolygon/minimal/consensus/ibft"
)

var consensusBackends = map[string]consensus.Factory{
	"dev":   consensusDev.Factory,
	"ibft":  consensusIBFT.Factory,
	"dummy": consensusDummy.Factory,
}

var consensusHubs = map[string]interface{}{
	"dev":   nil,
	"ibft":  newStakingHub(workingDirectory), // Returns a singleton
	"dummy": nil,
}
