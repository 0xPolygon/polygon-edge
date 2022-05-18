package helper

import (
	"crypto/ecdsa"
	"fmt"
	"path/filepath"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/awsssm"
	"github.com/0xPolygon/polygon-edge/secrets/gcpssm"
	"github.com/0xPolygon/polygon-edge/secrets/hashicorpvault"
	"github.com/0xPolygon/polygon-edge/secrets/local"
	"github.com/hashicorp/go-hclog"
	libp2pCrypto "github.com/libp2p/go-libp2p-core/crypto"
)

// SetupLocalSecretsManager is a helper method for boilerplate local secrets manager setup
func SetupLocalSecretsManager(dataDir string) (secrets.SecretsManager, error) {
	subDirectories := []string{secrets.ConsensusFolderLocal, secrets.NetworkFolderLocal}

	// Check if the sub-directories exist / are already populated
	for _, subDirectory := range subDirectories {
		if common.DirectoryExists(filepath.Join(dataDir, subDirectory)) {
			return nil,
				fmt.Errorf(
					"directory %s has previously initialized secrets data",
					dataDir,
				)
		}
	}

	return local.SecretsManagerFactory(
		nil, // Local secrets manager doesn't require a config
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
			Extra: map[string]interface{}{
				secrets.Path: dataDir,
			},
		},
	)
}

// SetupHashicorpVault is a helper method for boilerplate hashicorp vault secrets manager setup
func SetupHashicorpVault(
	secretsConfig *secrets.SecretsManagerConfig,
) (secrets.SecretsManager, error) {
	return hashicorpvault.SecretsManagerFactory(
		secretsConfig,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
		},
	)
}

// SetupAWSSSM is a helper method for boilerplate aws ssm secrets manager setup
func SetupAWSSSM(
	secretsConfig *secrets.SecretsManagerConfig,
) (secrets.SecretsManager, error) {
	return awsssm.SecretsManagerFactory(
		secretsConfig,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
		},
	)
}

// SetupGCPSSM is a helper method for boilerplate Google Cloud Computing secrets manager setup
func SetupGCPSSM(
	secretsConfig *secrets.SecretsManagerConfig,
) (secrets.SecretsManager, error) {
	return gcpssm.SecretsManagerFactory(
		secretsConfig,
		&secrets.SecretsManagerParams{
			Logger: hclog.NewNullLogger(),
		},
	)
}

func InitValidatorKey(secretsManager secrets.SecretsManager) (*ecdsa.PrivateKey, error) {
	// Generate the IBFT validator private key
	validatorKey, validatorKeyEncoded, keyErr := crypto.GenerateAndEncodePrivateKey()
	if keyErr != nil {
		return nil, keyErr
	}

	// Write the validator private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(
		secrets.ValidatorKey,
		validatorKeyEncoded,
	); setErr != nil {
		return nil, setErr
	}

	return validatorKey, nil
}

func InitNetworkingPrivateKey(secretsManager secrets.SecretsManager) (libp2pCrypto.PrivKey, error) {
	// Generate the libp2p private key
	libp2pKey, libp2pKeyEncoded, keyErr := network.GenerateAndEncodeLibp2pKey()
	if keyErr != nil {
		return nil, keyErr
	}

	// Write the networking private key to the secrets manager storage
	if setErr := secretsManager.SetSecret(
		secrets.NetworkKey,
		libp2pKeyEncoded,
	); setErr != nil {
		return nil, setErr
	}

	return libp2pKey, keyErr
}
