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

var consensusBackends = map[string]consensus.Factory{
	"dev":   consensusDev.Factory,
	"ibft":  consensusIBFT.Factory,
	"dummy": consensusDummy.Factory,
}

// secretsManagerBackends defines the SecretManager factories for different
// secret management solutions
var secretsManagerBackends = map[secrets.SecretsManagerType]secrets.SecretsManagerFactory{
	secrets.Local:          local.SecretsManagerFactory,
	secrets.HashicorpVault: hashicorpvault.SecretsManagerFactory,
}
