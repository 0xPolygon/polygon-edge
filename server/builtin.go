package server

import (
	"github.com/dogechain-lab/jury/consensus"
	consensusDev "github.com/dogechain-lab/jury/consensus/dev"
	consensusDummy "github.com/dogechain-lab/jury/consensus/dummy"
	consensusIBFT "github.com/dogechain-lab/jury/consensus/ibft"
	"github.com/dogechain-lab/jury/secrets"
	"github.com/dogechain-lab/jury/secrets/awsssm"
	"github.com/dogechain-lab/jury/secrets/hashicorpvault"
	"github.com/dogechain-lab/jury/secrets/local"
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
	secrets.AWSSSM:         awsssm.SecretsManagerFactory,
}

func ConsensusSupported(value string) bool {
	_, ok := consensusBackends[ConsensusType(value)]

	return ok
}
