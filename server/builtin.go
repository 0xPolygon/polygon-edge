package server

import (
	consensusDev "github.com/0xPolygon/polygon-sdk/consensus/dev"
	consensusDummy "github.com/0xPolygon/polygon-sdk/consensus/dummy"
	consensusIBFT "github.com/0xPolygon/polygon-sdk/consensus/ibft"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/local"

	"github.com/0xPolygon/polygon-sdk/consensus"
)

var consensusBackends = map[string]consensus.Factory{
	// "ethash": consensusEthash.Factory,
	"dev":   consensusDev.Factory,
	"ibft":  consensusIBFT.Factory,
	"dummy": consensusDummy.Factory,
}

// secretsManagerBackends defines the SecretManager factories for different
// secret management solutions
var secretsManagerBackends = map[string]secrets.SecretsManagerFactory{
	"local": local.SecretsManagerFactory,
	//"hashicorp-vault": {},
}
