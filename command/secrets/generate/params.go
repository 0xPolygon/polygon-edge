package generate

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/command/output"
	"github.com/0xPolygon/polygon-edge/secrets"
)

var (
	params = &generateParams{}
)

const (
	dirFlag       = "dir"
	tokenFlag     = "token"
	serverURLFlag = "server-url"
	typeFlag      = "type"
	nameFlag      = "name"
	namespaceFlag = "namespace"
)

const (
	defaultNodeName       = "polygon-edge-node"
	defaultConfigFileName = "./secretsManagerConfig.json"
	defaultNamespace      = "admin"
)

var (
	errUnsupportedType = errors.New("unsupported service manager type")
)

type generateParams struct {
	dir         string
	token       string
	serverURL   string
	serviceType string
	name        string
	namespace   string
}

func (p *generateParams) getRequiredFlags() []string {
	return []string{
		dirFlag,
		tokenFlag,
		serverURLFlag,
		nameFlag,
	}
}

func (p *generateParams) writeSecretsConfig() error {
	secretsConfig, err := p.generateSecretsConfig()
	if err != nil {
		return err
	}

	writeErr := secretsConfig.WriteConfig(p.dir)
	if writeErr != nil {
		return fmt.Errorf("unable to write configuration file, %w", writeErr)
	}

	return nil
}

func (p *generateParams) generateSecretsConfig() (*secrets.SecretsManagerConfig, error) {
	if !secrets.SupportedServiceManager(secrets.SecretsManagerType(p.serviceType)) {
		return nil, errUnsupportedType
	}

	// Generate the configuration
	return &secrets.SecretsManagerConfig{
		Token:     p.token,
		ServerURL: p.serverURL,
		Type:      secrets.SecretsManagerType(p.serviceType),
		Name:      p.name,
		Namespace: p.namespace,
		Extra:     nil,
	}, nil
}

func (p *generateParams) getResult() output.CommandResult {
	return &SecretsGenerateResult{
		ServiceType: p.serviceType,
		ServerURL:   p.serverURL,
		AccessToken: p.token,
		NodeName:    p.name,
		Namespace:   p.namespace,
	}
}
