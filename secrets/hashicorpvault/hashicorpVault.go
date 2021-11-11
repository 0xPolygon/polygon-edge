package hashicorpvault

import (
	"errors"

	"github.com/0xPolygon/polygon-sdk/secrets"
)

// VaultSecretsManager is a SecretsManager that
// stores secrets on a Hashicorp Vault instance
type VaultSecretsManager struct {
	// Token used for Vault instance authentication
	token string

	// The Server URL of the Vault instance
	serverURL string

	// The name of the current node, used for prefixing names of secrets
	name string
}

// SecretsManagerFactory implements the factory method
func SecretsManagerFactory(
	config *secrets.SecretsManagerParams,
) (secrets.SecretsManager, error) {
	// Set up the base object
	vaultManager := &VaultSecretsManager{}

	// Grab the token from the config
	token, ok := config.Params[secrets.Token]
	if !ok {
		return nil, errors.New("no token specified for Vault secrets manager")
	}
	vaultManager.token = token.(string)

	// Grab the server URL from the config
	serverURL, ok := config.Params[secrets.Server]
	if !ok {
		return nil, errors.New("no server URL specified for Vault secrets manager")
	}
	vaultManager.serverURL = serverURL.(string)

	// Grab the node name from the config
	name, ok := config.Params[secrets.Name]
	if !ok {
		return nil, errors.New("no node name specified for Vault secrets manager")
	}
	vaultManager.name = name.(string)

	// Run the initial setup
	_ = vaultManager.Setup()

	return vaultManager, nil
}

func (v *VaultSecretsManager) Setup() error {
	panic("implement me")
}

func (v *VaultSecretsManager) GetSecret(name string) (interface{}, error) {
	panic("implement me")
}

func (v *VaultSecretsManager) SetSecret(name string, value interface{}) error {
	panic("implement me")
}

func (v *VaultSecretsManager) HasSecret(name string) bool {
	panic("implement me")
}

func (v *VaultSecretsManager) RemoveSecret(name string) error {
	panic("implement me")
}
