package server

import (
	consensusDev "github.com/0xPolygon/polygon-edge/consensus/dev"
	consensusDummy "github.com/0xPolygon/polygon-edge/consensus/dummy"
	consensusIBFT "github.com/0xPolygon/polygon-edge/consensus/ibft"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-edge/secrets/local"

	"github.com/0xPolygon/polygon-edge/consensus"
)

type ConsensusType string

const (
	DevConsensus   ConsensusType = "dev"
	IBFTConsensus  ConsensusType = "ibft"
	DummyConsensus ConsensusType = "dummy"
)

var consensusBackends = map[ConsensusType]consensus.Factory{
	DevConsensus:   consensusDev.Factory,
	IBFTConsensus:  consensusIBFT.Factory,
	DummyConsensus: consensusDummy.Factory,
}

// secretsManagerBackends defines the SecretManager factories for different
// secret management solutions
var secretsManagerBackends = map[secrets.SecretsManagerType]secrets.SecretsManagerFactory{
	secrets.Local:          local.SecretsManagerFactory,
	secrets.HashicorpVault: hashicorpvault.SecretsManagerFactory,
}

func ConsensusSupported(value string) bool {
	_, ok := consensusBackends[ConsensusType(value)]

	return ok
}
