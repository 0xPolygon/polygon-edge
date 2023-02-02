package secrets

import (
	"encoding/json"
	"os"

	"github.com/0xPolygon/polygon-edge/helper/common"
)

// SecretsManagerConfig is the configuration that gets
// written to a single configuration file
type SecretsManagerConfig struct {
	Token     string                 `json:"token"`      // Access token to the instance
	ServerURL string                 `json:"server_url"` // The URL of the running server
	Type      SecretsManagerType     `json:"type"`       // The type of SecretsManager
	Name      string                 `json:"name"`       // The name of the current node
	Namespace string                 `json:"namespace"`  // The namespace of the service
	Extra     map[string]interface{} `json:"extra"`      // Any kind of arbitrary data
}

// WriteConfig writes the current configuration to the specified path
func (c *SecretsManagerConfig) WriteConfig(path string) error {
	jsonBytes, _ := json.MarshalIndent(c, "", " ")

	return common.SaveFileSafe(path, jsonBytes, 0660)
}

// ReadConfig reads the SecretsManagerConfig from the specified path
func ReadConfig(path string) (*SecretsManagerConfig, error) {
	configFile, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}

	config := &SecretsManagerConfig{}

	unmarshalErr := json.Unmarshal(configFile, &config)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}

	return config, nil
}
