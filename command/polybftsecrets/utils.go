package polybftsecrets

import (
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/secrets/helper"
)

// common flags for all polybft commands
const (
	AccountDirFlag    = "data-dir"
	AccountConfigFlag = "config"
	PrivateKeyFlag    = "private-key"
	ChainIDFlag       = "chain-id"

	AccountDirFlagDesc    = "the directory for the Polygon Edge data if the local FS is used"
	AccountConfigFlagDesc = "the path to the SecretsManager config file, if omitted, the local FS secrets manager is used"
	PrivateKeyFlagDesc    = "hex-encoded private key of the account which executes command"
	ChainIDFlagDesc       = "ID of child chain"
)

// common errors for all polybft commands
var (
	ErrInvalidNum                     = fmt.Errorf("num flag value should be between 1 and %d", maxInitNum)
	ErrInvalidParams                  = errors.New("no config file or data directory passed in")
	ErrUnsupportedType                = errors.New("unsupported secrets manager")
	ErrSecureLocalStoreNotImplemented = errors.New(
		"use a secrets backend, or supply an --insecure flag " +
			"to store the private keys locally on the filesystem, " +
			"avoid doing so in production")
)

// GetSecretsManager function resolves secrets manager instance based on provided data or config paths.
// insecureLocalStore defines if utilization of local secrets manager is allowed.
func GetSecretsManager(dataPath, configPath string, insecureLocalStore bool) (secrets.SecretsManager, error) {
	if configPath != "" {
		secretsConfig, readErr := secrets.ReadConfig(configPath)
		if readErr != nil {
			return nil, fmt.Errorf("invalid secrets configuration: %w", readErr)
		}

		if !secrets.SupportedServiceManager(secretsConfig.Type) {
			return nil, ErrUnsupportedType
		}

		return helper.InitCloudSecretsManager(secretsConfig)
	}

	// Storing secrets on a local file system should only be allowed with --insecure flag,
	// to raise awareness that it should be only used in development/testing environments.
	// Production setups should use one of the supported secrets managers
	if !insecureLocalStore {
		return nil, ErrSecureLocalStoreNotImplemented
	}

	return helper.SetupLocalSecretsManager(dataPath)
}
