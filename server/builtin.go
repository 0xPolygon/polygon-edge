package server

import (
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/consensus"
	consensusDev "github.com/0xPolygon/polygon-edge/consensus/dev"
	consensusDummy "github.com/0xPolygon/polygon-edge/consensus/dummy"
	consensusIBFT "github.com/0xPolygon/polygon-edge/consensus/ibft"
	consensusPolyBFT "github.com/0xPolygon/polygon-edge/consensus/polybft"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/awsssm"
	"github.com/0xPolygon/polygon-edge/secrets/gcpssm"
	"github.com/0xPolygon/polygon-edge/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-edge/secrets/local"
	"github.com/0xPolygon/polygon-edge/state"
)

type GenesisFactoryHook func(config *chain.Chain, engineName string) func(*state.Transition) error

type ConsensusType string

const (
	DevConsensus     ConsensusType = "dev"
	IBFTConsensus    ConsensusType = "ibft"
	PolyBFTConsensus ConsensusType = consensusPolyBFT.PolyBFTConsensusName
	DummyConsensus   ConsensusType = "dummy"
)

var consensusBackends = map[ConsensusType]consensus.Factory{
	DevConsensus:     consensusDev.Factory,
	IBFTConsensus:    consensusIBFT.Factory,
	PolyBFTConsensus: consensusPolyBFT.Factory,
	DummyConsensus:   consensusDummy.Factory,
}

// secretsManagerBackends defines the SecretManager factories for different
// secret management solutions
var secretsManagerBackends = map[secrets.SecretsManagerType]secrets.SecretsManagerFactory{
	secrets.Local:          local.SecretsManagerFactory,
	secrets.HashicorpVault: hashicorpvault.SecretsManagerFactory,
	secrets.AWSSSM:         awsssm.SecretsManagerFactory,
	secrets.GCPSSM:         gcpssm.SecretsManagerFactory,
}

var genesisCreationFactory = map[ConsensusType]GenesisFactoryHook{
	PolyBFTConsensus: consensusPolyBFT.GenesisPostHookFactory,
}

func ConsensusSupported(value string) bool {
	_, ok := consensusBackends[ConsensusType(value)]

	return ok
}
